package account

import (
	"context"
	"net/mail"
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
	BirthDate       *string   `json:"birth_date" bun:"birth_date"`
	ProfilePhotoSrc *string   `json:"profile_photo_src" bun:"profile_photo_src"`
	MemberSince     time.Time `json:"member_since" bun:"member_since"`
	HasPin          bool      `json:"has_pin" bun:"has_pin"`
}

// fetchMe: query yang sama dipakai Me() dan UpdateMe() (abis update, balikin data terbaru
// tanpa perlu request /me terpisah).
//
// birth_date di-cast ke "YYYY-MM-DD" polos via to_char() -- pola yang sama dipakai sudocore2
// (lihat master/member/member_services.go) biar FE gak perlu parse RFC3339, cukup *string biasa.
func fetchMe(ctx context.Context, db *bun.DB, memberID int64) (meResponse, error) {
	var data meResponse
	err := db.NewRaw(`
		SELECT
			mm.id, mm.code, mm.name, mm.phone_number, mm.email, mm.gender,
			to_char(mm.birth_date, 'YYYY-MM-DD') AS birth_date,
			mm.profile_photo_src,
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
// Field yang SENGAJA gak diikutin: member_type_id (2026-09-10: sekarang selalu
// auth.MemberTypeCustomerID buat member yang daftar sendiri lewat mobile -- nilainya konstan/gak
// informatif buat customer, gak perlu diekspos), tier_level (lihat TierSpending()), is_active
// (redundan -- kalau session-nya valid berarti pasti aktif, LoginOTP/LoginPin/ResetPin semua udah
// filter is_active=true), contact_name/created_by/updated_by/updated_at (gak relevan buat customer).
func (h *handler) Me(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	data, err := fetchMe(c.Context(), h.db, memberID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("failed to fetch account data"))
	}

	return c.JSON(res.Success().SetData(data))
}

type updateMeRequest struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Gender    string `json:"gender"`
	BirthDate string `json:"birth_date"`
}

// dateOnlyLayout: format JSON "YYYY-MM-DD" polos, SAMA PERSIS konvensi sudocore2
// (backend/modules/master/member/member_model.go, custom type DateOnly) -- FE gak perlu kirim
// jam/timezone buat field tanggal lahir.
const dateOnlyLayout = "2006-01-02"

// currentProfileFields: state existing email/gender/birth_date SEBELUM update -- dipakai
// UpdateMe() buat cek "udah pernah diisi atau belum" (lihat alasan di bawah).
type currentProfileFields struct {
	Email     *string `bun:"email"`
	Gender    *string `bun:"gender"`
	BirthDate *string `bun:"birth_date"`
}

// UpdateMe: edit profil member yang lagi login -- PROTECTED, member_id dari session token.
// `name`+`email`+`gender`+`birth_date` digabung di 1 endpoint (2026-10-13: gender/birth_date
// sempet dipisah jadi endpoint sendiri-sendiri, digabung lagi; email baru ditambah) -- field
// lain (profile_photo_src) punya endpoint sendiri, lihat UPDATE PHOTO.md (karena upload file
// beda alur dari form JSON biasa).
//
// `name` wajib, gak boleh kosong, SELALU bisa diubah berkali-kali (gak kena aturan "sekali isi"
// di bawah).
//
// `email`/`gender`/`birth_date` cuma bisa diisi **SEKALI** (2026-10-13) -- begitu kolomnya di
// database UDAH TERISI (bukan null), request APAPUN yang menyertakan field itu (nilai apa pun,
// termasuk nilai yang sama persis kayak yang udah tersimpan) DITOLAK dengan error jelas ("...
// has already been set and cannot be changed"), SEBELUM sampai nyentuh query UPDATE. Field yang
// masih null di database tetap bisa diisi lewat body ini. Kirim "" (atau gak sertain field-nya)
// SELALU dianggap "gak ada perubahan yang diminta" buat field itu -- aman dikirim apa pun
// kondisinya (baik yang masih null maupun yang udah terisi), gak pernah kena validasi ini.
func (h *handler) UpdateMe(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var req updateMeRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("invalid request body"))
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return c.JSON(res.SetCode(100).SetMessage("name is required"))
	}

	email := strings.TrimSpace(req.Email)
	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			return c.JSON(res.SetCode(100).SetMessage("invalid email format"))
		}
	}

	switch req.Gender {
	case "", "male", "female":
	default:
		return c.JSON(res.SetCode(100).SetMessage(`gender must be "male", "female", or empty`))
	}

	if req.BirthDate != "" {
		if _, err := time.Parse(dateOnlyLayout, req.BirthDate); err != nil {
			return c.JSON(res.SetCode(100).SetMessage("invalid date format, must be YYYY-MM-DD"))
		}
	}

	// cek state existing SEBELUM update -- field yang udah terisi di database gak boleh
	// disentuh lagi, apa pun nilai yang dikirim (termasuk kalau sama persis).
	var current currentProfileFields
	if err := h.db.NewRaw(
		`SELECT email, gender, to_char(birth_date, 'YYYY-MM-DD') AS birth_date FROM master_member WHERE id = ?`, memberID,
	).Scan(c.Context(), &current); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("failed to fetch current profile data"))
	}

	if email != "" && current.Email != nil {
		return c.JSON(res.SetCode(100).SetMessage("email has already been set and cannot be changed"))
	}
	if req.Gender != "" && current.Gender != nil {
		return c.JSON(res.SetCode(100).SetMessage("gender has already been set and cannot be changed"))
	}
	if req.BirthDate != "" && current.BirthDate != nil {
		return c.JSON(res.SetCode(100).SetMessage("birth_date has already been set and cannot be changed"))
	}

	// COALESCE(?, kolom): "" dikirim (atau gak dikirim) -> *string(nil) -> NULL di level SQL ->
	// COALESCE skip, pertahankan nilai lama. Kolom yang masih null dan field-nya diisi -> lolos
	// pengecekan di atas -> di sini ke-UPDATE normal.
	var emailPtr, genderPtr, birthDatePtr *string
	if email != "" {
		emailPtr = &email
	}
	if req.Gender != "" {
		g := req.Gender
		genderPtr = &g
	}
	if req.BirthDate != "" {
		birthDatePtr = &req.BirthDate
	}

	if _, err := h.db.NewRaw(
		`UPDATE master_member SET
			name = ?,
			email = COALESCE(?, email),
			gender = COALESCE(?, gender),
			birth_date = COALESCE(?, birth_date)
		WHERE id = ?`,
		name, emailPtr, genderPtr, birthDatePtr, memberID,
	).Exec(c.Context()); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("failed to update profile"))
	}

	data, err := fetchMe(c.Context(), h.db, memberID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("profile saved, but failed to fetch the latest data"))
	}

	return c.JSON(res.Success().SetData(data))
}
