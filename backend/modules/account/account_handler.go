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
	Balance(c fiber.Ctx) error
	Point(c fiber.Ctx) error
	BalanceHistory(c fiber.Ctx) error
	PointHistory(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

// meRow: hasil scan mentah dari query -- flat, field tier masih prefixed (TierLevel/TierName/
// TierStyleTemplate) karena hasil JOIN. Dikonversi ke meResponse (tier dikelompokin jadi 1
// objek) sebelum dikirim ke client -- field mentah/internal ini sengaja gak punya json tag.
type meRow struct {
	ID                int64     `bun:"id"`
	Code              string    `bun:"code"`
	Name              string    `bun:"name"`
	PhoneNumber       string    `bun:"phone_number"`
	Email             *string   `bun:"email"`
	Gender            *string   `bun:"gender"`
	ProfilePhotoSrc   *string   `bun:"profile_photo_src"`
	MemberSince       time.Time `bun:"member_since"`
	HasPin            bool      `bun:"has_pin"`
	TierLevel         int       `bun:"tier_level"`
	TierName          *string   `bun:"tier_name"`
	TierStyleTemplate *string   `bun:"tier_style_template"`
}

type tierInfo struct {
	Level         int     `json:"level"`
	Name          *string `json:"name"`
	StyleTemplate *string `json:"style_template"`
}

type meResponse struct {
	ID              int64     `json:"id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	PhoneNumber     string    `json:"phone_number"`
	Email           *string   `json:"email"`
	Gender          *string   `json:"gender"`
	ProfilePhotoSrc *string   `json:"profile_photo_src"`
	MemberSince     time.Time `json:"member_since"`
	HasPin          bool      `json:"has_pin"`
	Tier            tierInfo  `json:"tier"`
}

// Me: profil akun customer yang lagi login -- PROTECTED, member_id diambil dari session token
// (middleware.Auth), BUKAN dari body/param, sama pola kayak CreatePin/ChangePin/ResetPin. Read-only,
// gak ada endpoint update di sini (belum ada fitur "edit profil").
//
// `has_pin` di-derive via EXISTS ke mobile_member_pin -- dipakai app buat mutusin nunjukin
// prompt "aktifkan login PIN" atau nyembunyiin opsi "Login pakai PIN".
//
// `tier.name`/`tier.style_template` di-LEFT JOIN dari master_member_tier_setting_detail (match
// by level) -- LEFT JOIN, bukan INNER, karena tier_level di master_member selalu ada (default 1)
// tapi admin bisa aja belum pernah setup master_member_tier_setting_detail sama sekali, jadi
// dua field ini kudu bisa null tanpa bikin row master_member ilang dari hasil query. Field
// tier dikelompokin jadi 1 objek (bukan tier_level/tier_name/tier_style_template flat) --
// nama field-nya sengaja disamain persis kolom master_member_tier_setting_detail (level/name/
// style_template).
//
// Field yang SENGAJA gak diikutin: member_type_id (selalu kosong buat member yang daftar sendiri
// lewat mobile -- beda konsep dari tier.level di atas), is_active (redundan -- kalau session-nya
// valid berarti pasti aktif, LoginOTP/LoginPin/ResetPin semua udah filter is_active=true),
// contact_name/created_by/updated_by/updated_at (gak relevan buat customer).
func (h *handler) Me(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var row meRow
	err := h.db.NewRaw(`
		SELECT
			mm.id, mm.code, mm.name, mm.phone_number, mm.email, mm.gender, mm.profile_photo_src,
			mm.created_at AS member_since,
			EXISTS(SELECT 1 FROM mobile_member_pin mp WHERE mp.member_id = mm.id) AS has_pin,
			mm.tier_level,
			mmtsd.name AS tier_name,
			mmtsd.style_template AS tier_style_template
		FROM master_member mm
		LEFT JOIN master_member_tier_setting_detail mmtsd ON mmtsd.level = mm.tier_level
		WHERE mm.id = ?
	`, memberID).Scan(c.Context(), &row)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data akun"))
	}

	data := meResponse{
		ID:              row.ID,
		Code:            row.Code,
		Name:            row.Name,
		PhoneNumber:     row.PhoneNumber,
		Email:           row.Email,
		Gender:          row.Gender,
		ProfilePhotoSrc: row.ProfilePhotoSrc,
		MemberSince:     row.MemberSince,
		HasPin:          row.HasPin,
		Tier: tierInfo{
			Level:         row.TierLevel,
			Name:          row.TierName,
			StyleTemplate: row.TierStyleTemplate,
		},
	}

	return c.JSON(res.Success().SetData(data))
}
