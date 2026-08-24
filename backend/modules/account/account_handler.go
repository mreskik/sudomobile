package account

import (
	"context"
	"strings"
	"time"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type Handler interface {
	Me(c fiber.Ctx) error
	UpdateMe(c fiber.Ctx) error
	Balance(c fiber.Ctx) error
	Point(c fiber.Ctx) error
	BalanceHistory(c fiber.Ctx) error
	PointHistory(c fiber.Ctx) error
	TierList(c fiber.Ctx) error
	UpdatePhoto(c fiber.Ctx) error
	TierSpending(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

type meResponse struct {
	ID              int64     `json:"id" bun:"id"`
	Code            string    `json:"code" bun:"code"`
	Name            string    `json:"name" bun:"name"`
	PhoneNumber     string    `json:"phone_number" bun:"phone_number"`
	Email           *string   `json:"email" bun:"email"`
	Gender          *string   `json:"gender" bun:"gender"`
	ProfilePhotoSrc *string   `json:"profile_photo_src" bun:"profile_photo_src"`
	MemberSince     time.Time `json:"member_since" bun:"member_since"`
	HasPin          bool      `json:"has_pin" bun:"has_pin"`
}

// fetchMe: query yang sama dipakai Me() dan UpdateMe() (abis update, balikin data terbaru
// tanpa perlu request /me terpisah).
func fetchMe(ctx context.Context, db *bun.DB, memberID int64) (meResponse, error) {
	var data meResponse
	err := db.NewRaw(`
		SELECT
			mm.id, mm.code, mm.name, mm.phone_number, mm.email, mm.gender, mm.profile_photo_src,
			mm.created_at AS member_since,
			EXISTS(SELECT 1 FROM mobile_member_pin mp WHERE mp.member_id = mm.id) AS has_pin
		FROM master_member mm
		WHERE mm.id = ?
	`, memberID).Scan(ctx, &data)
	return data, err
}

// Me: profil akun customer yang lagi login -- PROTECTED, member_id diambil dari session token
// (middleware.Auth), BUKAN dari body/param, sama pola kayak CreatePin/ChangePin/ResetPin.
//
// `has_pin` di-derive via EXISTS ke mobile_member_pin -- dipakai app buat mutusin nunjukin
// prompt "aktifkan login PIN" atau nyembunyiin opsi "Login pakai PIN".
//
// SENGAJA gak ada info tier di sini (2026-08-21, sebelumnya sempet ada) -- dipindah ke
// TierSpending() (endpoint terpisah, "tier-spending"), digabung bareng info spending & jadwal
// evaluasi karena tier/spending/evaluasi itu satu concern yang sama (basisnya sama-sama dari
// master_member_tier_setting), sementara /me murni data identitas/profil yang jarang berubah.
//
// Field yang SENGAJA gak diikutin: member_type_id (selalu kosong buat member yang daftar sendiri
// lewat mobile), tier_level (lihat TierSpending()), is_active (redundan -- kalau session-nya
// valid berarti pasti aktif, LoginOTP/LoginPin/ResetPin semua udah filter is_active=true),
// contact_name/created_by/updated_by/updated_at (gak relevan buat customer).
func (h *handler) Me(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	data, err := fetchMe(c.Context(), h.db, memberID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data akun"))
	}

	return c.JSON(res.Success().SetData(data))
}

type updateMeRequest struct {
	Name   string `json:"name"`
	Gender string `json:"gender"`
}

// UpdateMe: edit profil member yang lagi login -- PROTECTED, member_id dari session token.
// SENGAJA baru `name`+`gender` doang buat sekarang (2026-08-21) -- field lain (email,
// profile_photo_src, dst) belum ada cara editnya di sini (profile_photo_src punya endpoint
// sendiri, lihat UPDATE PHOTO.md).
//
// `name` wajib, gak boleh kosong. `gender` opsional -- kirim "male"/"female" buat set, kirim
// string kosong "" buat CLEAR (balikin ke null) -- BUKAN "gak dikirim = gak berubah", field ini
// selalu diproses ulang tiap request (bukan partial-update per-field yang beda-beda).
func (h *handler) UpdateMe(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var req updateMeRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body request tidak valid"))
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return c.JSON(res.SetCode(100).SetMessage("name wajib diisi"))
	}

	var genderPtr *string
	switch req.Gender {
	case "":
		genderPtr = nil
	case "male", "female":
		g := req.Gender
		genderPtr = &g
	default:
		return c.JSON(res.SetCode(100).SetMessage(`gender wajib "male", "female", atau dikosongin`))
	}

	if _, err := h.db.NewRaw(
		`UPDATE master_member SET name = ?, gender = ? WHERE id = ?`, name, genderPtr, memberID,
	).Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal update profil"))
	}

	data, err := fetchMe(c.Context(), h.db, memberID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("profil kesimpen, tapi gagal ambil data terbaru"))
	}

	return c.JSON(res.Success().SetData(data))
}
