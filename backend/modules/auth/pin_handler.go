package auth

import (
	"database/sql"
	"errors"
	"time"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
)

type createPinRequest struct {
	Pin string `json:"pin"`
}

// CreatePin: bikin PIN (6 digit) buat member yang lagi login, CUMA buat pertama kali --
// PROTECTED, member_id diambil dari session token (middleware.Auth), BUKAN dari body request,
// biar user cuma bisa bikin PIN akun sendiri. 1 member cuma boleh punya 1 PIN (member_id
// UNIQUE di mobile_member_pin) -- kalau udah ada, DITOLAK (bukan di-update diam-diam kayak
// sebelumnya) -- ganti PIN yang udah ada wajib lewat endpoint change-pin terpisah (verifikasi
// PIN lama dulu), biar gak bisa diambil-alih cuma modal token session doang.
func (h *handler) CreatePin(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var req createPinRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body request tidak valid"))
	}

	hash, err := hashPin(req.Pin)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("pin harus 6 digit angka"))
	}

	var existing MobileMemberPin
	err = h.db.NewRaw(
		`SELECT id, member_id, pin_hash FROM mobile_member_pin WHERE member_id = ?`, memberID,
	).Scan(c.Context(), &existing)

	switch {
	case err == nil:
		return c.JSON(res.SetCode(100).SetMessage("pin sudah pernah dibuat, gunakan ganti pin"))
	case errors.Is(err, sql.ErrNoRows):
		newPin := MobileMemberPin{MemberID: memberID, PinHash: hash}
		if _, err := h.db.NewInsert().Model(&newPin).Exec(c.Context()); err != nil {
			return c.JSON(res.SetCode(100).SetMessage("gagal simpan pin"))
		}
	default:
		return c.JSON(res.SetCode(100).SetMessage("gagal cek pin"))
	}

	return c.JSON(res.Success().SetMessage("pin berhasil disimpan"))
}

type changePinRequest struct {
	OldPin string `json:"old_pin"`
	NewPin string `json:"new_pin"`
}

// ChangePin: ganti PIN yang UDAH ADA -- PROTECTED, member_id dari session token (sama kayak
// CreatePin). Beda dari CreatePin: wajib verifikasi old_pin dulu sebelum boleh ganti, karena
// ini ngubah credential yang udah ada (bukan bikin baru) -- gak boleh cuma modal token session
// doang, biar kalau token somehow ke-compromise, PIN tetep gak bisa diambil-alih tanpa tau
// PIN lama.
func (h *handler) ChangePin(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var req changePinRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body request tidak valid"))
	}

	newHash, err := hashPin(req.NewPin)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("pin baru harus 6 digit angka"))
	}

	var existing MobileMemberPin
	err = h.db.NewRaw(
		`SELECT id, member_id, pin_hash FROM mobile_member_pin WHERE member_id = ?`, memberID,
	).Scan(c.Context(), &existing)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("pin belum pernah dibuat, gunakan buat pin"))
		}
		return c.JSON(res.SetCode(100).SetMessage("gagal cek pin"))
	}

	if !comparePin(req.OldPin, existing.PinHash) {
		return c.JSON(res.SetCode(100).SetMessage("pin lama salah"))
	}

	now := time.Now()
	existing.PinHash = newHash
	existing.UpdatedAt = &now
	if _, err := h.db.NewUpdate().Model(&existing).Column("pin_hash", "updated_at").WherePK().Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal simpan pin"))
	}

	return c.JSON(res.Success().SetMessage("pin berhasil diganti"))
}

type resetPinRequest struct {
	PhoneNumber string `json:"phone_number"`
	OTP         string `json:"otp"`
	NewPin      string `json:"new_pin"`
}

// ResetPin: buat member yang LUPA PIN lama (gak bisa lewat ChangePin karena itu wajib tau PIN
// lama) -- PUBLIK (gak butuh token), tapi wajib bukti kepemilikan nomor lewat OTP FRESH
// (type=reset_pin), bukan token session lama. Kalau OTP valid, PIN langsung di-overwrite
// (upsert -- gak peduli udah ada PIN sebelumnya atau belum, beda dari CreatePin yang nolak
// kalau udah ada). Ikut pola Register/LoginOTP: sukses = langsung dapet session token baru
// juga, gak perlu login_pin terpisah lagi setelah reset.
func (h *handler) ResetPin(c fiber.Ctx) error {
	res := helpers.NewResponse()

	var req resetPinRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body request tidak valid"))
	}
	if req.PhoneNumber == "" {
		return c.JSON(res.SetCode(100).SetMessage("phone_number wajib diisi"))
	}
	if req.OTP == "" {
		return c.JSON(res.SetCode(100).SetMessage("otp wajib diisi"))
	}

	newHash, err := hashPin(req.NewPin)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("pin baru harus 6 digit angka"))
	}

	var member MasterMember
	err = h.db.NewRaw(
		`SELECT id, code, name, phone_number, is_active FROM master_member WHERE phone_number = ? AND is_active = true`,
		req.PhoneNumber,
	).Scan(c.Context(), &member)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("nomor belum terdaftar"))
		}
		return c.JSON(res.SetCode(100).SetMessage("gagal cek nomor"))
	}

	otp, err := findValidOTP(c.Context(), h.db, req.PhoneNumber, "reset_pin")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("otp tidak ditemukan atau sudah kedaluwarsa"))
		}
		return c.JSON(res.SetCode(100).SetMessage("gagal verifikasi otp"))
	}
	if otp.OTPCode != req.OTP {
		return c.JSON(res.SetCode(100).SetMessage("otp salah"))
	}

	token, err := generateSessionToken()
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal generate session"))
	}

	tx, err := h.db.BeginTx(c.Context(), nil)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal reset pin"))
	}
	gagal := true
	defer func() {
		if gagal {
			tx.Rollback()
		}
	}()

	now := time.Now()
	otp.VerifiedAt = &now
	if _, err := tx.NewUpdate().Model(&otp).Column("verified_at").WherePK().Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal reset pin"))
	}

	var existing MobileMemberPin
	errExisting := tx.NewRaw(
		`SELECT id, member_id, pin_hash FROM mobile_member_pin WHERE member_id = ?`, member.ID,
	).Scan(c.Context(), &existing)
	switch {
	case errExisting == nil:
		existing.PinHash = newHash
		existing.UpdatedAt = &now
		if _, err := tx.NewUpdate().Model(&existing).Column("pin_hash", "updated_at").WherePK().Exec(c.Context()); err != nil {
			return c.JSON(res.SetCode(100).SetMessage("gagal reset pin"))
		}
	case errors.Is(errExisting, sql.ErrNoRows):
		newPin := MobileMemberPin{MemberID: member.ID, PinHash: newHash}
		if _, err := tx.NewInsert().Model(&newPin).Exec(c.Context()); err != nil {
			return c.JSON(res.SetCode(100).SetMessage("gagal reset pin"))
		}
	default:
		return c.JSON(res.SetCode(100).SetMessage("gagal reset pin"))
	}

	session := MobileMemberSession{
		MemberID:  member.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(sessionExpiry),
	}
	if _, err := tx.NewInsert().Model(&session).Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal reset pin"))
	}

	gagal = false
	if err := tx.Commit(); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal reset pin"))
	}

	return c.JSON(res.Success().SetData(sessionResponse{
		Token: token,
		Member: memberResponse{
			ID:          member.ID,
			Code:        member.Code,
			Name:        member.Name,
			PhoneNumber: member.PhoneNumber,
		},
	}))
}

type loginPinRequest struct {
	PhoneNumber string `json:"phone_number"`
	Pin         string `json:"pin"`
}

// LoginPin: login pakai PIN -- alternatif LoginOTP, gak perlu nunggu kode OTP, tapi wajib
// udah pernah CreatePin duluan (butuh login lewat OTP dulu sekali). phone_number gak kedaftar
// / belum pernah set PIN / PIN salah, dibedain pesannya biar user paham posisinya.
func (h *handler) LoginPin(c fiber.Ctx) error {
	res := helpers.NewResponse()

	var req loginPinRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body request tidak valid"))
	}
	if req.PhoneNumber == "" {
		return c.JSON(res.SetCode(100).SetMessage("phone_number wajib diisi"))
	}
	if req.Pin == "" {
		return c.JSON(res.SetCode(100).SetMessage("pin wajib diisi"))
	}

	var member MasterMember
	err := h.db.NewRaw(
		`SELECT id, code, name, phone_number, is_active FROM master_member WHERE phone_number = ? AND is_active = true`,
		req.PhoneNumber,
	).Scan(c.Context(), &member)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("nomor belum terdaftar"))
		}
		return c.JSON(res.SetCode(100).SetMessage("gagal cek nomor"))
	}

	var pin MobileMemberPin
	err = h.db.NewRaw(
		`SELECT id, member_id, pin_hash FROM mobile_member_pin WHERE member_id = ?`, member.ID,
	).Scan(c.Context(), &pin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("pin belum pernah diset, silakan login pakai otp dulu"))
		}
		return c.JSON(res.SetCode(100).SetMessage("gagal cek pin"))
	}

	if !comparePin(req.Pin, pin.PinHash) {
		return c.JSON(res.SetCode(100).SetMessage("pin salah"))
	}

	token, err := generateSessionToken()
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal generate session"))
	}

	session := MobileMemberSession{
		MemberID:  member.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(sessionExpiry),
	}
	if _, err := h.db.NewInsert().Model(&session).Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal login"))
	}

	return c.JSON(res.Success().SetData(sessionResponse{
		Token: token,
		Member: memberResponse{
			ID:          member.ID,
			Code:        member.Code,
			Name:        member.Name,
			PhoneNumber: member.PhoneNumber,
		},
	}))
}
