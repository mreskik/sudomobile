package bestseller

import (
	"strconv"

	"sudomobile/backend/helpers"
	"sudomobile/backend/pricing"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type Handler interface {
	GetGlobal(c fiber.Ctx) error
	GetByBranch(c fiber.Ctx) error
	GetByVisitPurpose(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

// defaultLimit/maxLimit: default 10 item, client boleh minta lebih lewat ?limit= tapi di-cap 50
// biar gak nge-query balik semua menu.
const defaultLimit = 10
const maxLimit = 50

// SUMBER DATA: `mb_order`/`mb_order_detail` DOANG (2026-08-25, konfirmasi eksplisit) -- BUKAN
// gabung sama pos_order_detail. Konsisten sama keputusan scope promo sebelumnya (KETENTUAN
// PROMO.md, "scope pemakaian promo terpisah dari POS") -- best seller di sudomobile nunjukin
// popularitas DI MOBILE APP, bukan popularitas gabungan semua channel. Cuma order `status =
// 'paid'` yang dihitung (order pending/cancel/expired gak representasi penjualan beneran).
// Rentang waktu: 30 hari terakhir (trending, bukan all-time) -- HARDCODE, belum ada parameter
// buat ganti rentang.
//
// 3 endpoint beda SCOPE, bukan 3 implementasi terpisah -- makin sempit scope-nya, makin lengkap
// datanya (harga cuma bisa dikasih tau kalau menu_template_id-nya jelas, dan itu cuma bisa
// diresolve dari branch+visit_purpose, sama kayak GET VISIT PURPOSE DETAIL.md):
//   1. GetGlobal  -- semua branch/visit_purpose digabung. Gak ada harga (beda menu_template
//      bisa beda harga buat item yang sama, gak ada 1 angka yang "bener" buat ditampilin).
//   2. GetByBranch -- di-scope ke 1 branch, TETEP gak ada harga (1 branch bisa punya lebih dari
//      1 visit_purpose dengan menu_template beda-beda).
//   3. GetByVisitPurpose -- di-scope branch+visit_purpose, BARU ada harga (menu_template_id-nya
//      deterministik di titik ini, sama logic resolve yang dipakai Calculate()/menu-tree).

type bestSellerItemBase struct {
	MenuID      int64   `json:"menu_id" bun:"menu_id"`
	ItemName    *string `json:"item_name" bun:"item_name"`
	ImageSrc    *string `json:"image_src" bun:"image_src"`
	IconSrc     *string `json:"icon_src" bun:"icon_src"`
	TotalQty    int64   `json:"total_qty" bun:"total_qty"`
	TotalOrders int64   `json:"total_orders" bun:"total_orders"`
}

// GetGlobal: GET /api/menu/best-seller -- semua branch & visit_purpose digabung, 30 hari
// terakhir. PUBLIK (info agregat, gak nempel ke member manapun).
func (h *handler) GetGlobal(c fiber.Ctx) error {
	res := helpers.NewResponse()
	limit := parseLimit(c)

	list := []bestSellerItemBase{}
	err := h.db.NewRaw(`
		SELECT mod.menu_id, mi.item_name, mi.image AS image_src, mi.icon_src,
			SUM(mod.qty) AS total_qty, COUNT(DISTINCT mod.order_number) AS total_orders
		FROM mb_order_detail mod
		JOIN mb_order mo ON mo.order_number = mod.order_number
		LEFT JOIN master_item mi ON mi.id = mod.menu_id
		WHERE mo.status = 'paid' AND mo.created_at >= now() - interval '30 days'
		GROUP BY mod.menu_id, mi.item_name, mi.image, mi.icon_src
		ORDER BY total_qty DESC
		LIMIT ?
	`, limit).Scan(c.Context(), &list)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data best seller"))
	}

	return c.JSON(res.Success().SetData(list))
}

// GetByBranch: GET /api/branch/:branch_id/best-seller -- di-scope ke 1 branch.
func (h *handler) GetByBranch(c fiber.Ctx) error {
	res := helpers.NewResponse()

	branchID, err := strconv.Atoi(c.Params("branch_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("branch_id tidak valid"))
	}
	limit := parseLimit(c)

	list := []bestSellerItemBase{}
	err = h.db.NewRaw(`
		SELECT mod.menu_id, mi.item_name, mi.image AS image_src, mi.icon_src,
			SUM(mod.qty) AS total_qty, COUNT(DISTINCT mod.order_number) AS total_orders
		FROM mb_order_detail mod
		JOIN mb_order mo ON mo.order_number = mod.order_number
		LEFT JOIN master_item mi ON mi.id = mod.menu_id
		WHERE mo.status = 'paid' AND mo.created_at >= now() - interval '30 days' AND mo.branch_id = ?
		GROUP BY mod.menu_id, mi.item_name, mi.image, mi.icon_src
		ORDER BY total_qty DESC
		LIMIT ?
	`, branchID, limit).Scan(c.Context(), &list)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data best seller"))
	}

	return c.JSON(res.Success().SetData(list))
}

type bestSellerItemWithPrice struct {
	bestSellerItemBase
	Price     *string `json:"price" bun:"price"`
	TaxType   *string `json:"tax_type" bun:"tax_type"`
	TaxRate   *string `json:"tax_rate" bun:"tax_rate"`
	DPP       string  `json:"dpp"`
	NetDPP    string  `json:"net_dpp"`
	TaxAmount string  `json:"tax_amount"`
	Total     string  `json:"total"`
}

// GetByVisitPurpose: GET /api/branch/:branch_id/visit-purpose/:visit_purpose_id/best-seller --
// di-scope branch+visit_purpose, DITAMBAH harga (resolve menu_template_id + tax, sama logic
// yang dipakai pricing.CalculateLine() di Calculate()/menu-tree -- SATU sumber kebenaran buat
// perhitungan harga, gak ditulis ulang di sini).
func (h *handler) GetByVisitPurpose(c fiber.Ctx) error {
	res := helpers.NewResponse()

	branchID, err := strconv.Atoi(c.Params("branch_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("branch_id tidak valid"))
	}
	visitPurposeID, err := strconv.Atoi(c.Params("visit_purpose_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("visit_purpose_id tidak valid"))
	}
	limit := parseLimit(c)

	ctx := c.Context()

	cfg, err := pricing.ResolveVisitPurposeConfig(ctx, h.db, branchID, visitPurposeID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data visit purpose"))
	}
	if cfg == nil {
		return c.JSON(res.SetCode(100).SetMessage("visit purpose tidak ditemukan"))
	}

	taxRates, err := pricing.FetchTaxRates(ctx, h.db, cfg.ServiceCharge, cfg.Vat, cfg.Pb1)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data pajak"))
	}

	rows := []bestSellerItemWithPrice{}
	err = h.db.NewRaw(`
		SELECT mod.menu_id, mi.item_name, mi.image AS image_src, mi.icon_src, mi.use_tax AS tax_type,
			SUM(mod.qty) AS total_qty, COUNT(DISTINCT mod.order_number) AS total_orders,
			mpd.price
		FROM mb_order_detail mod
		JOIN mb_order mo ON mo.order_number = mod.order_number
		LEFT JOIN master_item mi ON mi.id = mod.menu_id
		LEFT JOIN master_item_conversion_detail micd ON micd.item_id = mi.id
		LEFT JOIN master_pricelist_detail mpd ON mpd.item_conversion_detail_id = micd.id
			AND mpd.menu_template_id = ? AND COALESCE(mpd.is_deleted, false) = false AND mpd.qr_order = true
		WHERE mo.status = 'paid' AND mo.created_at >= now() - interval '30 days'
			AND mo.branch_id = ? AND mo.visit_purpose_id = ?
		GROUP BY mod.menu_id, mi.item_name, mi.image, mi.icon_src, mi.use_tax, mpd.price
		ORDER BY total_qty DESC
		LIMIT ?
	`, cfg.MenuTemplateID, branchID, visitPurposeID, limit).Scan(ctx, &rows)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data best seller"))
	}

	for i := range rows {
		if rows[i].Price == nil {
			continue // item ini gak ketemu di menu_template ini lagi (mis. udah di-nonaktifin) -- harga gak diisi
		}
		useTax := ""
		if rows[i].TaxType != nil {
			useTax = *rows[i].TaxType
		}
		_, taxRate := pricing.ResolveItemTax(useTax, cfg, taxRates)
		calc, calcErr := pricing.CalculateLine(*rows[i].Price, taxRate, cfg.InclusivePrice, 0)
		if calcErr != nil {
			continue
		}
		rows[i].TaxRate = taxRate
		rows[i].DPP = calc.DPP
		rows[i].NetDPP = calc.NetDPP
		rows[i].TaxAmount = calc.TaxAmount
		rows[i].Total = calc.Total
	}

	return c.JSON(res.Success().SetData(rows))
}

func parseLimit(c fiber.Ctx) int {
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil || limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}
