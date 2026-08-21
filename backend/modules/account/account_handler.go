package account

import (
	"time"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type Handler interface {
	Me(c fiber.Ctx) error
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

// Me: profil akun customer yang lagi login -- PROTECTED, member_id diambil dari session token
// (middleware.Auth), BUKAN dari body/param, sama pola kayak CreatePin/ChangePin/ResetPin. Read-only,
// gak ada endpoint update di sini (belum ada fitur "edit profil").
//
// `has_pin` di-derive via EXISTS ke mobile_member_pin -- dipakai app buat mutusin nunjukin
// prompt "aktifkan login PIN" atau nyembunyiin opsi "Login pakai PIN".
//
// Field yang SENGAJA gak diikutin: member_type_id/nama tier (selalu kosong buat member yang
// daftar sendiri lewat mobile, belum ada mekanisme assign tier-nya), is_active (redundan --
// kalau session-nya valid berarti pasti aktif, LoginOTP/LoginPin/ResetPin semua udah filter
// is_active=true), contact_name/created_by/updated_by/updated_at (gak relevan buat customer).
func (h *handler) Me(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var data meResponse
	err := h.db.NewRaw(`
		SELECT
			mm.id, mm.code, mm.name, mm.phone_number, mm.email, mm.gender, mm.profile_photo_src,
			mm.created_at AS member_since,
			EXISTS(SELECT 1 FROM mobile_member_pin mp WHERE mp.member_id = mm.id) AS has_pin
		FROM master_member mm
		WHERE mm.id = ?
	`, memberID).Scan(c.Context(), &data)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data akun"))
	}

	return c.JSON(res.Success().SetData(data))
}
