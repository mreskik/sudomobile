package visitpurpose

import (
	"strconv"

	"sudomobile/backend/helpers"
	"sudomobile/backend/pricing"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type Handler interface {
	GetList(c fiber.Ctx) error
	GetDetail(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

type visitPurposeListItem struct {
	ID               int64  `json:"id" bun:"id"`
	VisitPurposeID   int64  `json:"visit_purpose_id" bun:"visit_purpose_id"`
	VisitPurposeName string `json:"visit_purpose_name" bun:"visit_purpose_name"`
}

// GetList: daftar visit purpose yang dibolehin muncul di mobile customer app buat 1 branch --
// PUBLIK (gak butuh Authorization), mirror KioskController::GetBranchVisitPurposeList() di POS
// tapi filter `flag_mobile_customer` (bukan `flag_kiosk`), dan `branch_id` EKSPLISIT di URL
// (2026-08-24, konfirmasi) -- beda dari Kiosk yang implisit (POS selalu 1 branch per install),
// sudomobile ngelayanin banyak branch sekaligus jadi wajib tau branch mana yang dimaksud.
//
// `id` -- id baris master_branch_visit_purpose itu sendiri (BUKAN yang dipakai buat manggil
// endpoint detail nanti). `visit_purpose_id` -- FK ke master_visit_purpose, INI yang dipakai
// buat detail (lihat GetDetail()).
//
// Filter: branch_id cocok, flag_mobile_customer=true, DAN is_active=true di kedua tabel
// (master_branch_visit_purpose & master_visit_purpose) -- beda dari versi Kiosk POS yang gak
// eksplisit ngecek is_active (POS-nya udah "aman" duluan lewat sync yang cuma narik baris
// aktif; sudomobile baca langsung ke ERP jadi perlu cek eksplisit sendiri).
func (h *handler) GetList(c fiber.Ctx) error {
	res := helpers.NewResponse()

	branchID, err := strconv.Atoi(c.Params("branch_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("branch_id tidak valid"))
	}

	list := []visitPurposeListItem{}
	err = h.db.NewRaw(`
		SELECT bvp.id, bvp.visit_purpose_id, vp.name AS visit_purpose_name
		FROM master_branch_visit_purpose bvp
		JOIN master_visit_purpose vp ON vp.id = bvp.visit_purpose_id
		WHERE bvp.branch_id = ? AND bvp.flag_mobile_customer = true
			AND bvp.is_active = true AND vp.is_active = true
		ORDER BY vp.name ASC
	`, branchID).Scan(c.Context(), &list)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data visit purpose"))
	}

	return c.JSON(res.Success().SetData(list))
}

type menuItemRow struct {
	CategoryID      int64   `bun:"category_id"`
	CategoryName    string  `bun:"category_name"`
	SubcategoryID   *int64  `bun:"subcategory_id"`
	SubcategoryName *string `bun:"subcategory_name"`
	IconSrc         *string `bun:"subcategory_icon_src"`
	BannerSrc       *string `bun:"subcategory_banner_src"`
	ItemID          int64   `bun:"item_id"`
	ItemCode        string  `bun:"item_code"`
	ItemName        string  `bun:"item_name"`
	ImageSrc        *string `bun:"image_src"`
	ItemIconSrc     *string `bun:"item_icon_src"`
	Price           string  `bun:"price"`
	UseTax          string  `bun:"use_tax"`
}

type menuItem struct {
	ItemID      int64                  `json:"item_id"`
	ItemCode    string                 `json:"item_code"`
	ItemName    string                 `json:"item_name"`
	ImageSrc    *string                `json:"image_src"`
	IconSrc     *string                `json:"icon_src"`
	Price       string                 `json:"price"`
	TaxType     string                 `json:"tax_type"`
	TaxID       *int64                 `json:"tax_id"`
	TaxRate     *string                `json:"tax_rate"`
	PackageList []pricing.PackageGroup `json:"package_list"`
}

type menuSubcategory struct {
	SubcategoryID   *int64     `json:"subcategory_id"`
	SubcategoryName *string    `json:"subcategory_name"`
	IconSrc         *string    `json:"icon_src"`
	BannerSrc       *string    `json:"banner_src"`
	Items           []menuItem `json:"items"`
}

type menuCategory struct {
	CategoryID    int64             `json:"category_id"`
	CategoryName  string            `json:"category_name"`
	Subcategories []menuSubcategory `json:"subcategories"`
}

type visitPurposeDetail struct {
	VisitPurposeID    int64          `json:"visit_purpose_id"`
	MenuTemplateID    int64          `json:"menu_template_id"`
	FlagInclusiveTax  bool           `json:"flag_inclusive_tax"`
	ServiceCharge     *int64         `json:"service_charge"`
	ServiceChargeRate *string        `json:"service_charge_rate"`
	Vat               *int64         `json:"vat"`
	VatRate           *string        `json:"vat_rate"`
	Pb1               *int64         `json:"pb1"`
	Pb1Rate           *string        `json:"pb1_rate"`
	OrderFee          *string        `json:"order_fee"`
	Categories        []menuCategory `json:"categories"`
}

// GetDetail: TAHAP 2 (2026-08-24) -- pohon menu + harga (Tahap 1) DITAMBAH resolusi pajak
// (service_charge/vat/pb1/order_fee level visit-purpose, + tax_type/tax_id/tax_rate per item).
// BELUM ada package/varian (Tahap 3). Mirror KioskController::GetBranchVisitPurposeDetail() di
// POS secara KONSEP -- versi POS reuse MenuServices::GetMasterMenuList() yang baca tabel lokal
// `mr_*` hasil sync, versi ini baca LANGSUNG skema ERP karena sudomobile connect ke DB yang
// sama kayak sudocore2, query & logic-nya ditulis ulang dari nol (skema beda total, ketemu
// lewat riset kode POS sebagai referensi "source of truth" formula-nya).
//
// Resolusi pajak per item (PERSIS niru MenuServices.php, BUKAN nebak sendiri):
//   - tax_type item = master_item.use_tax (BUKAN master_item_category.tax_type -- kolom itu
//     ada tapi TERNYATA gak dipakai di logic pajak manapun di seluruh codebase, cuma field
//     CRUD nganggur).
//   - use_tax == "vat" (persis, case-sensitive, gak ada trim) -> tax_id = vat milik visit
//     purpose. use_tax == "pb1" -> tax_id = pb1 milik visit purpose. Selain itu (termasuk
//     string kosong, atau typo kayak "pb 1" yang kebukti ada di data real) -> tax_id = null,
//     item DIANGGAP GAK KENA PAJAK. Ini bukan bug, ini PERSIS behavior POS.
//   - tax_id (dari visit purpose) yang nilainya 0 atau NULL -> tetep dianggap "gak ada pajak
//     jenis itu", walau use_tax-nya cocok.
//   - tax_rate -> lookup master_tax.rate pakai tax_id yang keresolve di atas. tax_id null ->
//     tax_rate null juga.
//   - flag_inclusive_tax -- SATU nilai buat SEMUA item (dari master_pricelist.inclusive_price,
//     level menu_template, bukan per item) -- makanya diletakin di level ATAS response
//     (bukan diulang di tiap item kayak POS, sengaja disederhanakan karena nilainya emang sama
//     semua).
//   - service_charge -- tax_id level visit purpose, SENGAJA cuma diresolve jadi rate (info
//     mentah), BELUM diaplikasikan ke perhitungan apapun -- riset ke kode POS gak nemu tempat
//     service_charge ini beneran dipakai buat ngitung harga (kemungkinan gap yang juga ada di
//     POS, atau logic-nya ada di tempat lain yang belum ketemu). Jangan diasumsikan "harga
//     final udah termasuk service charge" dari response ini.
func (h *handler) GetDetail(c fiber.Ctx) error {
	res := helpers.NewResponse()

	branchID, err := strconv.Atoi(c.Params("branch_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("branch_id tidak valid"))
	}
	visitPurposeID, err := strconv.Atoi(c.Params("visit_purpose_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("visit_purpose_id tidak valid"))
	}

	cfg, err := pricing.ResolveVisitPurposeConfig(c.Context(), h.db, branchID, visitPurposeID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data visit purpose"))
	}
	if cfg == nil {
		return c.JSON(res.SetCode(100).SetMessage("visit purpose tidak ditemukan"))
	}

	taxRates, err := pricing.FetchTaxRates(c.Context(), h.db, cfg.ServiceCharge, cfg.Vat, cfg.Pb1)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data pajak"))
	}

	// COALESCE(mpd.is_deleted, false) -- kolomnya NULLABLE dan di data real isinya NULL (bukan
	// literal false) buat baris yang emang gak dihapus, `is_deleted = false` doang gak match
	// NULL di SQL (ketauan pas tes live 2026-08-24), makanya di-COALESCE dulu.
	rows := []menuItemRow{}
	err = h.db.NewRaw(`
		SELECT
			mic.id AS category_id, mic.name AS category_name,
			misc.id AS subcategory_id, misc.name AS subcategory_name,
			misc.icon_src AS subcategory_icon_src, misc.banner_src AS subcategory_banner_src,
			mi.id AS item_id, mi.item_code, mi.item_name,
			mi.image AS image_src, mi.icon_src AS item_icon_src,
			mpd.price, mi.use_tax
		FROM master_pricelist_detail mpd
		JOIN master_item_conversion_detail micd ON micd.id = mpd.item_conversion_detail_id
		JOIN master_item mi ON mi.id = micd.item_id
		JOIN master_item_category mic ON mic.id = mi.item_category
		LEFT JOIN master_item_sub_category misc ON misc.id = mi.item_subcategory
		WHERE mpd.menu_template_id = ? AND COALESCE(mpd.is_deleted, false) = false AND mpd.qr_order = true
			AND mi.item_status = '1'
		ORDER BY mic.name ASC, misc.name ASC, mi.item_name ASC
	`, cfg.MenuTemplateID).Scan(c.Context(), &rows)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data menu"))
	}

	packages, err := pricing.FetchPackages(c.Context(), h.db, itemIDsOf(rows), cfg, taxRates)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data package"))
	}

	return c.JSON(res.Success().SetData(visitPurposeDetail{
		VisitPurposeID:    int64(visitPurposeID),
		MenuTemplateID:    cfg.MenuTemplateID,
		FlagInclusiveTax:  cfg.InclusivePrice,
		ServiceCharge:     cfg.ServiceCharge,
		ServiceChargeRate: taxRates.Rate(cfg.ServiceCharge),
		Vat:               cfg.Vat,
		VatRate:           taxRates.Rate(cfg.Vat),
		Pb1:               cfg.Pb1,
		Pb1Rate:           taxRates.Rate(cfg.Pb1),
		OrderFee:          cfg.OrderFee,
		Categories:        buildMenuTree(rows, cfg, taxRates, packages),
	}))
}

// itemIDsOf: daftar item_id UNIK dari hasil query menu (buat batch-query package sekali,
// bukan per-item).
func itemIDsOf(rows []menuItemRow) []int64 {
	ids := make([]int64, 0, len(rows))
	seen := map[int64]bool{}
	for _, row := range rows {
		if !seen[row.ItemID] {
			seen[row.ItemID] = true
			ids = append(ids, row.ItemID)
		}
	}
	return ids
}

// buildMenuTree: baris flat (1 baris = 1 item, kebawa nama category/subcategory-nya) dirakit
// jadi tree bersarang. Query udah ORDER BY category lalu subcategory lalu item name, jadi
// cukup "pecah begitu ID-nya ganti" -- gak perlu sort ulang di Go.
func buildMenuTree(rows []menuItemRow, cfg *pricing.VisitPurposeConfig, rates pricing.TaxRateMap, packages map[int64][]pricing.PackageGroup) []menuCategory {
	categories := []menuCategory{}

	for _, row := range rows {
		taxID, taxRate := pricing.ResolveItemTax(row.UseTax, cfg, rates)
		packageList, ok := packages[row.ItemID]
		if !ok {
			packageList = []pricing.PackageGroup{}
		}
		item := menuItem{
			ItemID:      row.ItemID,
			ItemCode:    row.ItemCode,
			ItemName:    row.ItemName,
			ImageSrc:    row.ImageSrc,
			IconSrc:     row.ItemIconSrc,
			Price:       row.Price,
			TaxType:     row.UseTax,
			TaxID:       taxID,
			TaxRate:     taxRate,
			PackageList: packageList,
		}

		catIdx := len(categories) - 1
		if catIdx < 0 || categories[catIdx].CategoryID != row.CategoryID {
			categories = append(categories, menuCategory{
				CategoryID:    row.CategoryID,
				CategoryName:  row.CategoryName,
				Subcategories: []menuSubcategory{},
			})
			catIdx = len(categories) - 1
		}

		subs := categories[catIdx].Subcategories
		subIdx := len(subs) - 1
		sameSubcategory := subIdx >= 0 && ((subs[subIdx].SubcategoryID == nil && row.SubcategoryID == nil) ||
			(subs[subIdx].SubcategoryID != nil && row.SubcategoryID != nil && *subs[subIdx].SubcategoryID == *row.SubcategoryID))
		if !sameSubcategory {
			subs = append(subs, menuSubcategory{
				SubcategoryID:   row.SubcategoryID,
				SubcategoryName: row.SubcategoryName,
				IconSrc:         row.IconSrc,
				BannerSrc:       row.BannerSrc,
				Items:           []menuItem{},
			})
			subIdx = len(subs) - 1
		}
		subs[subIdx].Items = append(subs[subIdx].Items, item)
		categories[catIdx].Subcategories = subs
	}

	return categories
}
