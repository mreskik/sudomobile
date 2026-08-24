package auth

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"

	"sudomobile/backend/helpers"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

const (
	otpExpiry       = 5 * time.Minute
	otpFreeRequests = 3               // jatah request_otp bebas per siklus, lihat RequestOTP()
	otpCooldown     = 5 * time.Minute // cooldown setelah otpFreeRequests kepake
	sessionExpiry   = 30 * 24 * time.Hour
)

type Handler interface {
	CheckNumber(c fiber.Ctx) error
	RequestOTP(c fiber.Ctx) error
	Register(c fiber.Ctx) error
	LoginOTP(c fiber.Ctx) error
	CreatePin(c fiber.Ctx) error
	ChangePin(c fiber.Ctx) error
	ResetPin(c fiber.Ctx) error
	LoginPin(c fiber.Ctx) error
	Logout(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

type checkNumberRequest struct {
	PhoneNumber string `json:"phone_number"`
}

type checkNumberResponse struct {
	PhoneNumber  string `json:"phone_number"`
	IsRegistered bool   `json:"is_registered"`
}

// CheckNumber: cek nomor HP udah kedaftar di master_member apa belum -- dipanggil app SEBELUM
// minta OTP, biar app bisa nentuin mau nunjukin layar "login" atau "daftar". phone_number gak
// kedaftar BUKAN error (code tetep 0), jawabannya di is_registered.
//
// SENGAJA gak ada normalisasi -- exact match apa adanya ke DB (convention: kode negara tanpa
// '+', "62812xxx"). Frontend WAJIB udah kirim dalam format itu; kalau kirim format lain
// (08xx/+62xx) ya gak bakal ketemu, itu disengaja biar gak ada 2 sisi (frontend & backend)
// yang sama-sama nyoba "nebak" format, ujung-ujungnya malah bentrok.
//
// Response selalu HTTP 200 -- sukses/gagal ditentuin dari `code` di body, sama convention
// yang dipakai sudocore2/APIANDORDER.
func (h *handler) CheckNumber(c fiber.Ctx) error {
	res := helpers.NewResponse()

	var req checkNumberRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body request tidak valid"))
	}

	if req.PhoneNumber == "" {
		return c.JSON(res.SetCode(100).SetMessage("phone_number wajib diisi"))
	}

	var count int
	err := h.db.NewRaw(
		`SELECT COUNT(*) FROM master_member WHERE phone_number = ? AND is_active = true`,
		req.PhoneNumber,
	).Scan(c.Context(), &count)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal cek nomor"))
	}

	return c.JSON(res.Success().SetData(checkNumberResponse{
		PhoneNumber:  req.PhoneNumber,
		IsRegistered: count > 0,
	}))
}

type requestOTPRequest struct {
	PhoneNumber string `json:"phone_number"`
	Type        string `json:"type"`
}

type requestOTPResponse struct {
	PhoneNumber      string    `json:"phone_number"`
	Type             string    `json:"type"`
	ExpiresInSeconds int       `json:"expires_in_seconds"`
	ExpiresAt        time.Time `json:"expires_at"`
}

// RequestOTP: generate & simpen kode OTP buat phone_number -- `type` WAJIB "register", "login",
// atau "reset_pin", nentuin OTP ini boleh dipakai di endpoint mana (Register cuma nerima
// type=register, LoginOTP cuma nerima type=login, ResetPin cuma nerima type=reset_pin -- gak
// bisa ketuker/disalahgunain). SEMENTARA belum ada
// provider WA/SMS -- kode-nya cuma disimpen ke mobile_member_otp & di-log ke console, dicek
// langsung dari database pas development.
// Kode LAMA buat phone_number+type yang sama TETEP ada di tabel (gak dihapus/di-invalidate) --
// konsumennya (Register/LoginOTP) nyari yang PALING BARU & masih valid, jadi otomatis yang
// lama gak kepake lagi begitu ada yang baru, gak perlu cleanup eksplisit di sini.
func (h *handler) RequestOTP(c fiber.Ctx) error {
	res := helpers.NewResponse()

	var req requestOTPRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body request tidak valid"))
	}
	if req.PhoneNumber == "" {
		return c.JSON(res.SetCode(100).SetMessage("phone_number wajib diisi"))
	}
	if req.Type != "register" && req.Type != "login" && req.Type != "reset_pin" {
		return c.JSON(res.SetCode(100).SetMessage(`type wajib "register", "login", atau "reset_pin"`))
	}

	// Barier GLOBAL per phone_number (lintas type, bukan per-type) -- gonta-ganti type buat
	// dapet OTP baru gak boleh ngelewatin limit ini. Bukan cooldown tiap request -- request
	// ke-1 & ke-2 dalam 1 siklus SELALU bebas (gak ada cek waktu sama sekali). Begitu nyampe
	// otpFreeRequests (3), baris ke-3 itu jadi acuan cooldown: request ke-4+ wajib nunggu
	// otpCooldown (5 menit) dari request TERAKHIR. Begitu cooldown-nya kelewatan, siklus baru
	// dimulai lagi dari request_seq=1 (BUKAN reset harian/kalender) -- jadi dapet lagi jatah 2x
	// bebas sebelum kena cooldown berikutnya.
	var last struct {
		CreatedAt  time.Time `bun:"created_at"`
		RequestSeq int       `bun:"request_seq"`
	}
	err := h.db.NewRaw(
		`SELECT created_at, request_seq FROM mobile_member_otp WHERE phone_number = ? ORDER BY id DESC LIMIT 1`,
		req.PhoneNumber,
	).Scan(c.Context(), &last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return c.JSON(res.SetCode(100).SetMessage("gagal cek otp"))
	}

	nextSeq := 1
	if err == nil {
		if last.RequestSeq < otpFreeRequests {
			nextSeq = last.RequestSeq + 1
		} else if remaining := otpCooldown - time.Since(last.CreatedAt); remaining > 0 {
			return c.JSON(res.SetCode(100).
				SetMessage("terlalu sering minta otp, coba lagi nanti").
				SetData(fiber.Map{"retry_after_seconds": int(remaining.Seconds())}))
		}
		// else: cooldown udah kelewatan -- siklus baru, nextSeq tetep 1.
	}

	code, err := generateOTPCode()
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal generate otp"))
	}

	otp := MobileMemberOTP{
		PhoneNumber: req.PhoneNumber,
		OTPCode:     code,
		Type:        req.Type,
		ExpiresAt:   time.Now().Add(otpExpiry),
		RequestSeq:  nextSeq,
	}
	if _, err := h.db.NewInsert().Model(&otp).Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal generate otp"))
	}

	// TODO: kirim beneran lewat provider WA/SMS begitu udah dipilih -- sekarang cuma di-log.
	log.Println("[DEV] OTP", req.Type, "buat", req.PhoneNumber, "=", code, "(expired", otpExpiry, ")")

	return c.JSON(res.Success().SetData(requestOTPResponse{
		PhoneNumber:      req.PhoneNumber,
		Type:             req.Type,
		ExpiresInSeconds: int(otpExpiry.Seconds()),
		ExpiresAt:        otp.ExpiresAt,
	}))
}

// findValidOTP: ambil baris mobile_member_otp PALING BARU buat phone_number+type ini yang
// masih valid (belum expired, belum pernah diverifikasi) -- dipake bareng Register & LoginOTP.
func findValidOTP(ctx context.Context, db bun.IDB, phoneNumber, otpType string) (MobileMemberOTP, error) {
	var otp MobileMemberOTP
	err := db.NewRaw(`
		SELECT id, phone_number, otp_code, type, expires_at, verified_at
		FROM mobile_member_otp
		WHERE phone_number = ? AND type = ? AND verified_at IS NULL AND expires_at > now()
		ORDER BY id DESC
		LIMIT 1
	`, phoneNumber, otpType).Scan(ctx, &otp)
	return otp, err
}

type registerRequest struct {
	PhoneNumber string `json:"phone_number"`
	Name        string `json:"name"`
	OTP         string `json:"otp"`
}

type memberResponse struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	PhoneNumber string `json:"phone_number"`
	HasPin      bool   `json:"has_pin"`
}

// hasPin: dipakai bareng Register/LoginOTP/LoginPin/ResetPin buat ngisi memberResponse.HasPin --
// sama query persis kayak account.Me() (EXISTS ke mobile_member_pin). Duplikat query sengaja
// (bukan import cross-package) -- auth & account itu module terpisah dalam 1 binary yang sama,
// belum ada shared package internal buat query kecil kayak gini.
func hasPin(ctx context.Context, db bun.IDB, memberID int64) (bool, error) {
	var has bool
	err := db.NewRaw(
		`SELECT EXISTS(SELECT 1 FROM mobile_member_pin WHERE member_id = ?)`, memberID,
	).Scan(ctx, &has)
	return has, err
}

type sessionResponse struct {
	Token  string         `json:"token"`
	Member memberResponse `json:"member"`
}

// Register: bikin master_member baru dari phone_number+name, SETELAH otp-nya divalidasi --
// bukan cuma buat "verify OTP" doang, langsung create akun + terbitin session token sekalian
// (register di app ini = 1 langkah, gak ada step "verify" terpisah dari "create akun"). OTP-nya
// WAJIB type="register" -- OTP yang diminta buat login gak bisa dipakai daftar.
func (h *handler) Register(c fiber.Ctx) error {
	res := helpers.NewResponse()

	var req registerRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body request tidak valid"))
	}
	if req.PhoneNumber == "" {
		return c.JSON(res.SetCode(100).SetMessage("phone_number wajib diisi"))
	}
	if req.Name == "" {
		return c.JSON(res.SetCode(100).SetMessage("name wajib diisi"))
	}
	if req.OTP == "" {
		return c.JSON(res.SetCode(100).SetMessage("otp wajib diisi"))
	}

	// re-cek belum kedaftar -- check_number di app cuma snapshot pas awal, antara itu & pas
	// user beneran submit register bisa aja nomor yang sama udah didaftarin duluan (device
	// lain / request duplikat), jadi wajib dicek ulang di sini, bukan percaya begitu aja ke
	// hasil check_number sebelumnya.
	var existingCount int
	if err := h.db.NewRaw(
		`SELECT COUNT(*) FROM master_member WHERE phone_number = ?`, req.PhoneNumber,
	).Scan(c.Context(), &existingCount); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal cek nomor"))
	}
	if existingCount > 0 {
		return c.JSON(res.SetCode(100).SetMessage("nomor sudah terdaftar"))
	}

	otp, err := findValidOTP(c.Context(), h.db, req.PhoneNumber, "register")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("otp tidak ditemukan atau sudah kedaluwarsa"))
		}
		return c.JSON(res.SetCode(100).SetMessage("gagal verifikasi otp"))
	}
	if otp.OTPCode != req.OTP {
		return c.JSON(res.SetCode(100).SetMessage("otp salah"))
	}

	code, err := generateMemberCode(c.Context(), h.db)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal generate member code"))
	}

	token, err := generateSessionToken()
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal generate session"))
	}

	tx, err := h.db.BeginTx(c.Context(), nil)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal register"))
	}
	gagal := true
	defer func() {
		if gagal {
			tx.Rollback()
		}
	}()

	member := MasterMember{
		Code:        code,
		Name:        req.Name,
		PhoneNumber: req.PhoneNumber,
		IsActive:    true,
	}
	if _, err := tx.NewInsert().Model(&member).Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal register"))
	}

	now := time.Now()
	otp.VerifiedAt = &now
	if _, err := tx.NewUpdate().Model(&otp).Column("verified_at").WherePK().Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal register"))
	}

	session := MobileMemberSession{
		MemberID:  member.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(sessionExpiry),
	}
	if _, err := tx.NewInsert().Model(&session).Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal register"))
	}

	gagal = false
	if err := tx.Commit(); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal register"))
	}

	return c.JSON(res.Success().SetData(sessionResponse{
		Token: token,
		Member: memberResponse{
			ID:          member.ID,
			Code:        member.Code,
			Name:        member.Name,
			PhoneNumber: member.PhoneNumber,
			HasPin:      false, // member baru daftar -- mustahil udah punya PIN, gak perlu query
		},
	}))
}

type loginOTPRequest struct {
	PhoneNumber string `json:"phone_number"`
	OTP         string `json:"otp"`
}

// loginOTPMemberResponse/loginOTPResponse: struct RESPONSE SENDIRI (2026-08-21), gak share
// memberResponse/sessionResponse kayak Register/LoginPin/ResetPin -- sengaja dipisah karena
// has_pin di endpoint ini SEMANTIKNYA beda (beneran di-query, bisa true/false), sementara di
// 3 endpoint lain nilainya selalu ketebak/literal (lihat komentar di masing-masing handler).
type loginOTPMemberResponse struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	PhoneNumber string `json:"phone_number"`
	HasPin      bool   `json:"has_pin"`
}

type loginOTPResponse struct {
	Token  string                 `json:"token"`
	Member loginOTPMemberResponse `json:"member"`
}

// LoginOTP: login pakai OTP buat nomor yang UDAH kedaftar -- kebalikan Register (gak ada
// `name`, gak insert master_member baru, cuma cari yang udah ada & terbitin session baru).
// OTP-nya WAJIB type="login" -- OTP yang diminta buat register gak bisa dipakai login.
func (h *handler) LoginOTP(c fiber.Ctx) error {
	res := helpers.NewResponse()

	var req loginOTPRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body request tidak valid"))
	}
	if req.PhoneNumber == "" {
		return c.JSON(res.SetCode(100).SetMessage("phone_number wajib diisi"))
	}
	if req.OTP == "" {
		return c.JSON(res.SetCode(100).SetMessage("otp wajib diisi"))
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

	otp, err := findValidOTP(c.Context(), h.db, req.PhoneNumber, "login")
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
		return c.JSON(res.SetCode(100).SetMessage("gagal login"))
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
		return c.JSON(res.SetCode(100).SetMessage("gagal login"))
	}

	session := MobileMemberSession{
		MemberID:  member.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(sessionExpiry),
	}
	if _, err := tx.NewInsert().Model(&session).Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal login"))
	}

	gagal = false
	if err := tx.Commit(); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal login"))
	}

	// beda dari Register/LoginPin/ResetPin -- login lewat OTP gak ngewajibin punya PIN, jadi
	// has_pin di sini beneran bisa true atau false, wajib dicek, gak bisa diasumsikan.
	memberHasPin, err := hasPin(c.Context(), h.db, member.ID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal cek status pin"))
	}

	return c.JSON(res.Success().SetData(loginOTPResponse{
		Token: token,
		Member: loginOTPMemberResponse{
			ID:          member.ID,
			Code:        member.Code,
			Name:        member.Name,
			PhoneNumber: member.PhoneNumber,
			HasPin:      memberHasPin,
		},
	}))
}

// Logout: hapus session yang lagi dipakai (token dari header Authorization request ini) --
// SATU device doang, bukan semua session milik member (sama filosofi kayak ResetPin: device
// lain yang masih login tetep login, gak ke-revoke ikut-ikutan). Hard delete, bukan soft
// delete/revoked_at -- konsisten sama gaya modul lain di service ini yang belum butuh audit
// trail buat tabel session.
//
// PROTECTED (middleware.Auth) -- tapi di sini butuh TOKEN mentahnya sendiri (bukan cuma
// member_id yang udah divalidasi & disimpen middleware ke locals), jadi header Authorization
// di-parse ulang di sini. middleware.Auth udah mastiin token ini valid sebelum sampai ke
// handler, jadi gak perlu validasi ulang -- tinggal delete baris yang match.
func (h *handler) Logout(c fiber.Ctx) error {
	res := helpers.NewResponse()

	token := strings.TrimPrefix(c.Get("Authorization"), "Bearer ")

	if _, err := h.db.NewRaw(
		`DELETE FROM mobile_member_session WHERE token = ?`, token,
	).Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal logout"))
	}

	return c.JSON(res.Success())
}
