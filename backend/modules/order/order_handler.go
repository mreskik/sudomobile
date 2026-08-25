package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"
	"sudomobile/backend/pricing"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type Handler interface {
	Calculate(c fiber.Ctx) error
	Create(c fiber.Ctx) error
	CheckPaymentStatus(c fiber.Ctx) error
	CancelOrder(c fiber.Ctx) error
	GetHistory(c fiber.Ctx) error
	GetDetail(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

// selectionRequest/packageRequest/itemRequest/calculateRequest: bentuk body PERSIS yang bakal
// dipakai juga di POST /api/order (save order) nanti -- SENGAJA dibikin sama biar frontend bisa
// pakai payload yang sama persis buat Calculate() (preview) dan Create() (submit beneran), cuma
// beda endpoint. `price`/`tax_id`/dll SENGAJA gak ada field-nya di sini -- client cuma kirim
// identitas + qty, server yang resolve harga/pajak dari DB (gak percaya angka dari client).
type selectionRequest struct {
	MenuPackageID int64 `json:"menu_package_id"`
	Qty           int64 `json:"qty"`
}

type packageRequest struct {
	PackageID  int64              `json:"package_id"`
	Selections []selectionRequest `json:"selections"`
}

type itemRequest struct {
	MenuID   int64            `json:"menu_id"`
	Qty      int64            `json:"qty"`
	Notes    string           `json:"notes"`
	Packages []packageRequest `json:"packages"`
}

// calculateRequest.UsePromoIDs: SEJAJAR sama items (level order), BUKAN nested di tiap item --
// client cuma bilang "promo mana yang mau dipakai", server yang nyari sendiri baris item mana
// di cart yang cocok jadi target-nya (lewat promo_for: category/subcategory/item), client gak
// perlu tau logic matching-nya sama sekali (2026-08-24, konfirmasi/redesign dari versi awal
// yang promo_id-nya nested per-item). 1 promo bisa kena ke LEBIH DARI 1 baris kalau emang
// banyak yang match (misal target category, cart ada beberapa item category itu) -- tapi 2
// promo yang REBUTAN baris yang sama (sama-sama match ke 1 item) DITOLAK sebagai ambigu, bukan
// di-first-match-wins diam-diam. Field-nya dinamain `use_promo_ids` (bukan `promo_ids` doang) --
// biar lebih eksplisit ini "promo yang mau DIPAKAI", bukan "daftar promo yang tersedia"
// (2026-08-25, permintaan eksplisit).
type calculateRequest struct {
	BranchID       int           `json:"branch_id"`
	VisitPurposeID int           `json:"visit_purpose_id"`
	Items          []itemRequest `json:"items"`
	UsePromoIDs    []int64       `json:"use_promo_ids"`
}

// menuRow: hasil query resolveMenuRows() -- 1 baris = 1 item YANG DIMINTA client, dibatasi ke
// item_id yang diminta doang (beda dari visitpurpose_handler.GetDetail yang narik SEMUA item di
// menu_template) -- lebih murah karena payload order biasanya cuma beberapa item, bukan seluruh
// menu.
type menuRow struct {
	ItemID            int64  `bun:"item_id"`
	ItemName          string `bun:"item_name"`
	CategoryID        *int64 `bun:"category_id"`
	SubcategoryID     *int64 `bun:"subcategory_id"`
	PricelistDetailID int64  `bun:"pricelist_detail_id"`
	Price             string `bun:"price"`
	UseTax            string `bun:"use_tax"`
}

type calculatedPackageItem struct {
	MenuPackageID int64   `json:"menu_package_id"`
	ItemID        int64   `json:"item_id"`
	ItemName      string  `json:"item_name"`
	Qty           int64   `json:"qty"`
	Price         string  `json:"price"`
	TaxType       string  `json:"tax_type"`
	TaxID         *int64  `json:"tax_id"`
	TaxRate       *string `json:"tax_rate"`
	DPP           string  `json:"dpp"`
	NetDPP        string  `json:"net_dpp"`
	TaxAmount     string  `json:"tax_amount"`
	Total         string  `json:"total"`
}

type calculatedItem struct {
	MenuID            int64                   `json:"menu_id"`
	ItemName          string                  `json:"item_name"`
	PricelistDetailID int64                   `json:"pricelist_detail_id"`
	CategoryID        *int64                  `json:"category_id"`
	SubcategoryID     *int64                  `json:"subcategory_id"`
	Qty               int64                   `json:"qty"`
	Notes             string                  `json:"notes"`
	Price             string                  `json:"price"`
	TaxType           string                  `json:"tax_type"`
	TaxID             *int64                  `json:"tax_id"`
	TaxRate           *string                 `json:"tax_rate"`
	DPP               string                  `json:"dpp"`
	NetDPP            string                  `json:"net_dpp"`
	TaxAmount         string                  `json:"tax_amount"`
	Total             string                  `json:"total"`
	Packages          []calculatedPackageItem `json:"packages"`
	PromoID           *int64                  `json:"promo_id"`
	PromoName         *string                 `json:"promo_name"`
	DiscountPercent   string                  `json:"discount_percent"`
	DiscountAmount    string                  `json:"discount_amount"`
}

type calculateResult struct {
	VisitPurposeID   int64            `json:"visit_purpose_id"`
	MenuTemplateID   int64            `json:"menu_template_id"`
	FlagInclusiveTax bool             `json:"flag_inclusive_tax"`
	Items            []calculatedItem `json:"items"`
	SubTotal         string           `json:"sub_total"`
	TotalTax         string           `json:"total_tax"`
	TotalDiscount    string           `json:"total_discount"`
	TotalBilling     string           `json:"total_billing"`
}

// Calculate: preview breakdown harga/pajak SEBELUM order beneran disubmit -- TANPA insert apa
// pun ke mb_order* (baca-only). Body SAMA PERSIS kayak yang bakal dipakai POST /api/order
// (save order) nanti, sengaja begitu biar frontend bisa reuse payload yang sama antara
// "hitung dulu di keranjang" dan "submit order beneran".
//
// PENTING: logic resolve harga/pajak/validasi di sini dipakai ULANG persis (lewat package
// pricing + resolveMenuRows/validateAndCalculate di file ini) oleh Create() (save order) nanti
// -- SATU fungsi buat kedua endpoint, BUKAN 2 implementasi terpisah yang keliatan sama. Ini
// sengaja biar harga preview di keranjang gak pernah beda sama harga final pas checkout.
func (h *handler) Calculate(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var body calculateRequest
	if err := c.Bind().Body(&body); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body tidak valid"))
	}
	if body.BranchID == 0 || body.VisitPurposeID == 0 {
		return c.JSON(res.SetCode(100).SetMessage("branch_id dan visit_purpose_id wajib diisi"))
	}
	if len(body.Items) == 0 {
		return c.JSON(res.SetCode(100).SetMessage("items tidak boleh kosong"))
	}

	result, errMsg, err := calculateOrder(c.Context(), h.db, body, memberID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal menghitung order"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	return c.JSON(res.Success().SetData(result))
}

// calculateOrder: fungsi inti, DIPISAH dari handler Calculate() supaya bisa dipanggil ulang
// sama handler Create() (save order, belum dibangun) tanpa duplikasi. Balikin (result, "",
// nil) kalau sukses, (nil, "pesan error validasi", nil) kalau body-nya invalid secara bisnis
// (item gak ketemu, qty package di luar min/max, dst -- ini BUKAN error server), atau
// (nil, "", err) kalau beneran error DB.
func calculateOrder(ctx context.Context, db *bun.DB, body calculateRequest, memberID int64) (*calculateResult, string, error) {
	cfg, err := pricing.ResolveVisitPurposeConfig(ctx, db, body.BranchID, body.VisitPurposeID)
	if err != nil {
		return nil, "", err
	}
	if cfg == nil {
		return nil, "visit purpose tidak ditemukan", nil
	}

	menuIDs := make([]int64, 0, len(body.Items))
	for _, item := range body.Items {
		menuIDs = append(menuIDs, item.MenuID)
	}

	menuRows, err := resolveMenuRows(ctx, db, cfg.MenuTemplateID, menuIDs)
	if err != nil {
		return nil, "", err
	}

	taxRates, err := pricing.FetchTaxRates(ctx, db, cfg.ServiceCharge, cfg.Vat, cfg.Pb1)
	if err != nil {
		return nil, "", err
	}

	packages, err := pricing.FetchPackages(ctx, db, menuIDs, cfg, taxRates)
	if err != nil {
		return nil, "", err
	}

	// hasPromo: cuma fetch data member (member_type_id + saldo poin) kalau BENERAN ada promo yang
	// diminta -- member_id selalu ada (endpoint ini protected), tapi query tambahan itu gak
	// gratis, sayang dijalanin kalau gak ada promo yang diminta sama sekali.
	var memberTypeID int64
	var memberPoint float64
	if len(body.UsePromoIDs) > 0 {
		memberTypeID, memberPoint, err = fetchMemberPromoContext(ctx, db, memberID)
		if err != nil {
			return nil, "", err
		}
	}

	result := &calculateResult{
		VisitPurposeID:   int64(body.VisitPurposeID),
		MenuTemplateID:   cfg.MenuTemplateID,
		FlagInclusiveTax: cfg.InclusivePrice,
		Items:            []calculatedItem{},
	}

	// PASS 1: hitung harga/pajak TANPA diskon dulu buat semua item -- dipakai buat nentuin
	// pre-discount subtotal (acuan cek min_buy_amount promo), sebelum promo diterapkan di PASS 2.
	type pendingItem struct {
		req  itemRequest
		row  menuRow
		calc pricing.LineCalculation
	}
	pending := make([]pendingItem, 0, len(body.Items))
	preDiscountSubtotal := 0.0

	for _, itemReq := range body.Items {
		if itemReq.Qty <= 0 {
			return nil, "qty item wajib lebih dari 0", nil
		}
		row, ok := menuRows[itemReq.MenuID]
		if !ok {
			return nil, "item tidak ditemukan di menu branch/visit purpose ini", nil
		}

		_, taxRate := pricing.ResolveItemTax(row.UseTax, cfg, taxRates)
		calc, err := pricing.CalculateLine(row.Price, taxRate, cfg.InclusivePrice, 0)
		if err != nil {
			return nil, "", err
		}
		preDiscountSubtotal += float64(itemReq.Qty) * mustFloat(calc.DPP)
		pending = append(pending, pendingItem{req: itemReq, row: row, calc: calc})
	}

	// Resolusi promo_ids (level ORDER, sejajar items) -- server yang nyari sendiri baris item
	// mana yang cocok jadi target tiap promo (BUKAN client yang nunjuk). assignedPromo:
	// index di `pending` -> promo yang kena ke baris itu (nil kalau gak ada).
	assignedPromo := make([]*pricing.Promo, len(pending))
	for _, promoID := range body.UsePromoIDs {
		promo, dErr := pricing.ResolvePromo(ctx, db, promoID, body.BranchID, body.VisitPurposeID, memberTypeID)
		if dErr != nil {
			return nil, "", dErr
		}
		if promo == nil {
			return nil, fmt.Sprintf("promo %d tidak ditemukan / tidak berlaku", promoID), nil
		}

		minBuy := mustFloat(promo.MinBuyAmount)
		if minBuy > 0 && preDiscountSubtotal < minBuy {
			return nil, fmt.Sprintf("belanja belum mencapai minimum buat promo %d", promoID), nil
		}
		minPoint := mustFloat(promo.MinPointAmount)
		if minPoint > 0 && memberPoint < minPoint {
			return nil, fmt.Sprintf("poin member gak cukup buat promo %d", promoID), nil
		}
		usedToday, dErr := pricing.PromoUsedToday(ctx, db, promo.ID)
		if dErr != nil {
			return nil, "", dErr
		}
		if promo.ApplyLimitPerDay != nil && *promo.ApplyLimitPerDay > 0 && usedToday >= *promo.ApplyLimitPerDay {
			return nil, fmt.Sprintf("promo %d udah mencapai limit pemakaian hari ini", promoID), nil
		}

		matchedAny := false
		for idx, p := range pending {
			matches, dErr := pricing.PromoTargetMatches(ctx, db, promo.ID, promo.PromoFor, p.req.MenuID, p.row.CategoryID, p.row.SubcategoryID)
			if dErr != nil {
				return nil, "", dErr
			}
			if !matches {
				continue
			}
			matchedAny = true
			if existing := assignedPromo[idx]; existing != nil {
				return nil, fmt.Sprintf("promo %d dan %d sama-sama cocok ke item %s -- pilih salah satu", existing.ID, promo.ID, p.row.ItemName), nil
			}
			assignedPromo[idx] = promo
		}
		if !matchedAny {
			return nil, fmt.Sprintf("promo %d tidak berlaku buat item apa pun di cart", promoID), nil
		}
	}

	subTotal, totalTax, totalBilling, totalDiscount := 0.0, 0.0, 0.0, 0.0

	for idx, p := range pending {
		itemReq, row := p.req, p.row
		taxID, taxRate := pricing.ResolveItemTax(row.UseTax, cfg, taxRates)

		discountAmount := 0.0
		discountPercent := "0.00"
		var promoID *int64
		var promoName *string
		if promo := assignedPromo[idx]; promo != nil {
			var dErr error
			discountAmount, dErr = pricing.CalculatePromoDiscount(promo, mustFloat(p.calc.DPP))
			if dErr != nil {
				return nil, dErr.Error(), nil
			}
			promoID = &promo.ID
			promoName = &promo.Name
			// discount_percent cuma keisi kalau tipe promo-nya emang "percent" (rate ASLI
			// promo, bukan dihitung balik dari discount_amount/dpp) -- promo "rupiah" nyimpen
			// 0.00, sama kayak POS yang nyimpen discount_percent/discount_amount sebagai 2
			// representasi independen, bukan saling nurunin.
			if promo.Type == "percent" && promo.TypePercentRate != nil {
				discountPercent = *promo.TypePercentRate
			}
		}

		calc, err := pricing.CalculateLine(row.Price, taxRate, cfg.InclusivePrice, discountAmount)
		if err != nil {
			return nil, "", err
		}

		calcItem := calculatedItem{
			MenuID:            itemReq.MenuID,
			ItemName:          row.ItemName,
			PricelistDetailID: row.PricelistDetailID,
			CategoryID:        row.CategoryID,
			SubcategoryID:     row.SubcategoryID,
			Qty:               itemReq.Qty,
			Notes:             itemReq.Notes,
			Price:             row.Price,
			TaxType:           row.UseTax,
			TaxID:             taxID,
			TaxRate:           taxRate,
			DPP:               calc.DPP,
			NetDPP:            calc.NetDPP,
			TaxAmount:         calc.TaxAmount,
			Total:             calc.Total,
			Packages:          []calculatedPackageItem{},
			PromoID:           promoID,
			PromoName:         promoName,
			DiscountPercent:   discountPercent,
			DiscountAmount:    formatFloat(discountAmount),
		}

		qtyF := float64(itemReq.Qty)
		subTotal += qtyF * mustFloat(calc.DPP)
		totalTax += qtyF * mustFloat(calc.TaxAmount)
		totalBilling += qtyF * mustFloat(calc.Total)
		totalDiscount += discountAmount

		availableGroups := map[int64]pricing.PackageGroup{}
		for _, g := range packages[itemReq.MenuID] {
			availableGroups[g.PackageID] = g
		}

		for _, pkgReq := range itemReq.Packages {
			group, ok := availableGroups[pkgReq.PackageID]
			if !ok {
				return nil, "package tidak ditemukan buat item ini", nil
			}

			subItemsByID := map[int64]pricing.PackageSubItem{}
			for _, sub := range group.MenuPackageList {
				subItemsByID[sub.MenuPackageID] = sub
			}

			var selectedQty int64
			for _, sel := range pkgReq.Selections {
				if sel.Qty <= 0 {
					return nil, "qty pilihan package wajib lebih dari 0", nil
				}
				subItem, ok := subItemsByID[sel.MenuPackageID]
				if !ok {
					return nil, "pilihan package tidak ditemukan di grup ini", nil
				}
				selectedQty += sel.Qty

				pkgCalc, err := pricing.CalculateLine(subItem.Price, subItem.TaxRate, cfg.InclusivePrice, 0)
				if err != nil {
					return nil, "", err
				}

				calcItem.Packages = append(calcItem.Packages, calculatedPackageItem{
					MenuPackageID: subItem.MenuPackageID,
					ItemID:        subItem.ItemID,
					ItemName:      subItem.ItemName,
					Qty:           sel.Qty,
					Price:         subItem.Price,
					TaxType:       subItem.TaxType,
					TaxID:         subItem.TaxID,
					TaxRate:       subItem.TaxRate,
					DPP:           pkgCalc.DPP,
					NetDPP:        pkgCalc.NetDPP,
					TaxAmount:     pkgCalc.TaxAmount,
					Total:         pkgCalc.Total,
				})

				pkgQtyF := qtyF * float64(sel.Qty)
				subTotal += pkgQtyF * mustFloat(pkgCalc.DPP)
				totalTax += pkgQtyF * mustFloat(pkgCalc.TaxAmount)
				totalBilling += pkgQtyF * mustFloat(pkgCalc.Total)
			}

			if selectedQty < group.MinQty || selectedQty > group.MaxQty {
				return nil, "jumlah pilihan package di luar batas min/max grup", nil
			}
		}

		result.Items = append(result.Items, calcItem)
	}

	result.SubTotal = formatFloat(subTotal)
	result.TotalTax = formatFloat(totalTax)
	result.TotalDiscount = formatFloat(totalDiscount)
	result.TotalBilling = formatFloat(totalBilling)

	return result, "", nil
}

// fetchMemberPromoContext: member_type_id + saldo poin TERBARU member, dipakai buat validasi
// eligibility promo (master_promo_type_members / min_point_amount). Query poin MIRROR PERSIS
// account/balance_handler.go::Point() -- baris terbaru member_point_ledger, "0" kalau belum
// pernah ada transaksi poin sama sekali (bukan error).
func fetchMemberPromoContext(ctx context.Context, db *bun.DB, memberID int64) (int64, float64, error) {
	memberTypeID, err := pricing.FetchMemberTypeID(ctx, db, memberID)
	if err != nil {
		return 0, 0, err
	}

	var point string
	err = db.NewRaw(`
		SELECT balance_after FROM member_point_ledger
		WHERE member_id = ? AND is_deleted = false
		ORDER BY created_at DESC, id DESC LIMIT 1
	`, memberID).Scan(ctx, &point)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, 0, err
		}
		point = "0"
	}

	return memberTypeID, mustFloat(point), nil
}

// resolveMenuRows: query 1x buat SEMUA menu_id yang diminta sekaligus (bukan per-item, biar
// gak N+1) -- balikin map item_id -> menuRow. item_id yang gak ketemu (di luar scope
// menu_template_id ini, atau item_status inactive, dst) SIMPLY GAK ADA di map -- pemanggil yang
// mutusin itu error validasi.
func resolveMenuRows(ctx context.Context, db *bun.DB, menuTemplateID int64, itemIDs []int64) (map[int64]menuRow, error) {
	result := map[int64]menuRow{}
	if len(itemIDs) == 0 {
		return result, nil
	}

	rows := []menuRow{}
	err := db.NewRaw(`
		SELECT
			mi.id AS item_id, mi.item_name, mi.item_category AS category_id, mi.item_subcategory AS subcategory_id,
			mpd.id AS pricelist_detail_id, mpd.price, mi.use_tax
		FROM master_pricelist_detail mpd
		JOIN master_item_conversion_detail micd ON micd.id = mpd.item_conversion_detail_id
		JOIN master_item mi ON mi.id = micd.item_id
		WHERE mpd.menu_template_id = ? AND COALESCE(mpd.is_deleted, false) = false AND mpd.qr_order = true
			AND mi.item_status = '1' AND mi.id IN (?)
	`, menuTemplateID, bun.In(itemIDs)).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.ItemID] = row
	}
	return result, nil
}
