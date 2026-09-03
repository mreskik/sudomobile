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
	ID   int64  `bun:"id"`
	Name string `bun:"name"`
}

type defaultBrandResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// GetDefault: brand aktif dari X-App-Setting (middleware.BrandID) -- BUKAN pilihan user, brand_id
// itu udah tetap per app-instance/build (lihat middleware/app_setting.go), endpoint ini cuma
// nerjemahin id itu jadi id+name yang siap ditampilin (mis. header/splash screen), gak perlu
// mobile app hardcode nama brand sendiri. PUBLIK (gak butuh Authorization) -- sama kayak
// GetBanners/branch.GetList, brand context udah ada dari X-App-Setting, bukan dari identitas
// member yang login. brand_id di X-App-Setting udah divalidasi eksistensinya di middleware
// (SELECT COUNT(*) FROM master_brand), jadi di sini row-nya seharusnya SELALU ketemu -- error
// "gak ketemu" cuma bisa kejadian kalau brand-nya dihapus tepat di antara validasi middleware &
// query ini (race yang sangat kecil kemungkinannya, tetep dihandle biar gak panic).
func (h *handler) GetDefault(c fiber.Ctx) error {
	res := helpers.NewResponse()
	brandID := middleware.BrandID(c)

	var row defaultBrandRow
	err := h.db.NewRaw(`SELECT id, name FROM master_brand WHERE id = ?`, brandID).Scan(c.Context(), &row)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data brand"))
	}

	return c.JSON(res.Success().SetData(defaultBrandResponse{
		ID:   row.ID,
		Name: row.Name,
	}))
}
