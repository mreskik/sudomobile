package brand

import (
	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type Handler interface {
	GetDefault(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

type defaultBrandRow struct {
	ID             int64  `bun:"id"`
	Name           string `bun:"name"`
	BrandColor     string `bun:"brand_color"`
	PrimaryColor   string `bun:"primary_color"`
	SecondaryColor string `bun:"secondary_color"`
}

type defaultBrandResponse struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	BrandColor     string `json:"brand_color"`
	PrimaryColor   string `json:"primary_color"`
	SecondaryColor string `json:"secondary_color"`
}

// GetDefault: brand aktif dari X-App-Setting (middleware.BrandID) -- BUKAN pilihan user, brand_id
// itu udah tetap per app-instance/build (lihat middleware/app_setting.go), endpoint ini cuma
// nerjemahin id itu jadi id+name+warna yang siap ditampilin (mis. header/splash screen, theming
// warna tombol/aksen), gak perlu mobile app hardcode nama/warna brand sendiri. PUBLIK (gak butuh
// Authorization) -- sama kayak GetBanners/branch.GetList, brand context udah ada dari
// X-App-Setting, bukan dari identitas member yang login. brand_id di X-App-Setting udah
// divalidasi eksistensinya di middleware (SELECT COUNT(*) FROM master_brand), jadi di sini
// row-nya seharusnya SELALU ketemu -- error "gak ketemu" cuma bisa kejadian kalau brand-nya
// dihapus tepat di antara validasi middleware & query ini (race yang sangat kecil
// kemungkinannya, tetep dihandle biar gak panic).
//
// brand_color/primary_color/secondary_color (2026-10-09) -- ganti dari theme_color tunggal yang
// lama di sudocore2 (migration 263, lihat MASTER BRAND.md sudocore2), baca langsung kolom yang
// sama di DB yang sama (sudomobile gak punya migration sendiri, connect ke DB sudocore2 -- lihat
// komentar package). Nullable di DB (brand lama yang belum diisi warnanya) -> COALESCE ke string
// kosong "", konsisten sama pola string lain di project ini (bukan nullable *string).
func (h *handler) GetDefault(c fiber.Ctx) error {
	res := helpers.NewResponse()
	brandID := middleware.BrandID(c)

	var row defaultBrandRow
	err := h.db.NewRaw(`
		SELECT id, name,
		COALESCE(brand_color, '') as brand_color,
		COALESCE(primary_color, '') as primary_color,
		COALESCE(secondary_color, '') as secondary_color
		FROM master_brand WHERE id = ?
	`, brandID).Scan(c.Context(), &row)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("failed to fetch brand data"))
	}

	return c.JSON(res.Success().SetData(defaultBrandResponse{
		ID:             row.ID,
		Name:           row.Name,
		BrandColor:     row.BrandColor,
		PrimaryColor:   row.PrimaryColor,
		SecondaryColor: row.SecondaryColor,
	}))
}
