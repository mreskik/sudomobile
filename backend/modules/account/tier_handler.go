package account

import (
	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
)

type tierListRow struct {
	Level          int     `json:"level" bun:"level"`
	Name           string  `json:"name" bun:"name"`
	SpendingAmount string  `json:"spending_amount" bun:"spending_amount"`
	StyleTemplate  *string `json:"style_template" bun:"style_template"`
	IsCurrent      bool    `json:"is_current" bun:"-"`
}

// TierList: daftar SEMUA level tier yang terdaftar (master_member_tier_setting_detail),
// ORDER BY level ASC -- bukan cuma tier member yang lagi login doang, biar app bisa render
// "roadmap"/"road to next tier" (nunjukin semua level + syarat spending-nya sekaligus, bukan
// cuma 1 level yang lagi ditempatin). `is_current` ditandain di baris yang level-nya cocok
// sama master_member.tier_level milik session yang lagi login -- biar app gak perlu nyocokin
// sendiri di sisi client.
//
// PROTECTED (bukan publik) -- sengaja disamain pola sama endpoint account/* lain, walau
// isinya sendiri bukan data pribadi (cuma `is_current` yang personal, daftar tier-nya sendiri
// sama buat semua orang).
func (h *handler) TierList(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var currentLevel int
	if err := h.db.NewRaw(
		`SELECT tier_level FROM master_member WHERE id = ?`, memberID,
	).Scan(c.Context(), &currentLevel); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data tier"))
	}

	var list []tierListRow
	if err := h.db.NewRaw(`
		SELECT level, name, spending_amount, style_template
		FROM master_member_tier_setting_detail
		ORDER BY level ASC
	`).Scan(c.Context(), &list); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil daftar tier"))
	}

	for i := range list {
		list[i].IsCurrent = list[i].Level == currentLevel
	}

	return c.JSON(res.Success().SetData(list))
}
