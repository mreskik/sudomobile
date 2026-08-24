package account

import (
	"sudomobile/backend/helpers"

	"github.com/gofiber/fiber/v3"
)

type tierListRow struct {
	Level          int     `json:"level" bun:"level"`
	Name           string  `json:"name" bun:"name"`
	SpendingAmount string  `json:"spending_amount" bun:"spending_amount"`
	StyleTemplate  *string `json:"style_template" bun:"style_template"`
}

// TierList: daftar SEMUA level tier yang terdaftar (master_member_tier_setting_detail),
// ORDER BY level ASC -- dipakai app buat render "roadmap"/"road to next tier". Sengaja gak ada
// `is_current`/gak nyentuh master_member sama sekali -- daftar tier itu sama buat semua orang,
// biar gak perlu extra JOIN/query di sini, app yang nyocokin sendiri di sisi client (bandingin
// ke `tier.level` dari ME.md).
//
// PROTECTED (bukan publik) -- sengaja disamain pola sama endpoint account/* lain, walau isinya
// sendiri bukan data pribadi.
func (h *handler) TierList(c fiber.Ctx) error {
	res := helpers.NewResponse()

	var list []tierListRow
	if err := h.db.NewRaw(`
		SELECT level, name, spending_amount, style_template
		FROM master_member_tier_setting_detail
		ORDER BY level ASC
	`).Scan(c.Context(), &list); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil daftar tier"))
	}

	return c.JSON(res.Success().SetData(list))
}
