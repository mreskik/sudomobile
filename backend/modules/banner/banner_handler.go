package banner

import (
	"context"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type Handler interface {
	GetBanners(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

// headerBanners: dibangun manual dari hasil fetchHeaderColumn()/fetchHeaderImageWithLink()
// terpisah (masing-masing PASANGAN bisa dari campaign berbeda satu sama lain) -- bukan hasil 1
// query yang di-scan langsung, jadi gak ada bun tag.
//
// QuickActionLeft/Right (gambar+link, 2026-10-06) SENGAJA fetch bareng dari fetchHeaderImageWithLink()
// -- BEDA dari Splash/LoginSheet (gak punya link pasangan, tetep fetchHeaderColumn() independen).
// Gambar dan link quick action HARUS dari CAMPAIGN YANG SAMA (link "menempel" ke gambar
// pasangannya) -- kalau campaign sumber gambar gak isi link, link-nya tetep null, BUKAN turun
// cari campaign lain yang ngisi link (itu bakal bikin gambar+link "ketukar campaign", link bisa
// nyasar ke campaign promosi yang udah gak relevan). Lihat fetchHeaderImageWithLink().
type headerBanners struct {
	BannerSplashSrc                 *string
	BannerQuickActionLeftButtonSrc  *string
	BannerQuickActionLeftLink       *string
	BannerQuickActionRightButtonSrc *string
	BannerQuickActionRightLink      *string
	BannerLoginSheetSrc             *string
}

// Sequence SENGAJA gak ikut di struct/response -- cuma dipakai buat ORDER BY di query
// (fetchNamedBanners/fetchPopupBanners/fetchScheduledPromotionBanners), datanya udah kekirim
// urut, frontend gak perlu tau angka mentahnya.
type namedBanner struct {
	BannerSrc string `json:"banner_src" bun:"banner_src"`
	Name      string `json:"name" bun:"name"`
}

type popupBanner struct {
	BannerSrc  string  `json:"banner_src" bun:"banner_src"`
	ActionLink *string `json:"action_link" bun:"action_link"`
}

// promotionBanner: SAMA PERSIS namedBanner + ActionLink (migration sudocore2 257, 2026-10-06) --
// BEDA struct dari namedBanner (bukan reuse) karena banner_swipe TIDAK punya action_link, cuma
// banner_promotion yang punya -- kalau direuse, banner_swipe juga keikut punya field action_link
// yang gak ada artinya.
type promotionBanner struct {
	BannerSrc  string  `json:"banner_src" bun:"banner_src"`
	Name       string  `json:"name" bun:"name"`
	ActionLink *string `json:"action_link" bun:"action_link"`
}

type bannerResponse struct {
	BannerSplashSrc                 *string            `json:"banner_splash_src"`
	BannerQuickActionLeftButtonSrc  *string            `json:"banner_quick_action_left_button_src"`
	BannerQuickActionLeftLink       *string            `json:"banner_quick_action_left_link"`
	BannerQuickActionRightButtonSrc *string            `json:"banner_quick_action_right_button_src"`
	BannerQuickActionRightLink      *string            `json:"banner_quick_action_right_link"`
	BannerLoginSheetSrc             *string            `json:"banner_login_sheet_src"`
	BannerSwipe                     []namedBanner      `json:"banner_swipe"`
	BannerPopup                     []popupBanner      `json:"banner_popup"`
	BannerPromotion                 []promotionBanner  `json:"banner_promotion"`
	BannerAboutUs                   []namedBanner      `json:"banner_about_us"`
}

// scopeFilter: klausa WHERE + JOIN yang sama dipakai di semua query modul ini -- campaign aktif
// (is_active) DAN cocok scope brand (flag_all_brand=true ATAU ada baris di
// master_image_mb_cust_brands buat brand_id ini). Sama filosofi kayak master_image
// (branch-scoped) di sudocore2/APIANDORDER, cuma brand_id gantiin branch_id.
const scopeJoin = `LEFT JOIN master_image_mb_cust_brands mimcbr
		ON mimcbr.master_image_mb_cust_id = mimc.id AND mimcbr.brand_id = ?`
const scopeWhere = `mimc.is_active = true AND (mimc.flag_all_brand = true OR mimcbr.id IS NOT NULL)`

// dateScopeWhere: filter tanggal aktif per CAMPAIGN (2026-08-24, kolomnya di header
// master_image_mb_cust, BUKAN di tabel banner-nya) -- dipakai pas narik banner_swipe/
// banner_popup/banner_promotion (BUKAN banner_about_us, dan BUKAN 4 slot gambar tunggal
// header yang tetep pilih 1 campaign terbaru apa adanya). flag_all_date=false TAPI
// date_start/date_end NULL (belum diisi admin) sengaja dianggap TIDAK aktif (gak masuk kondisi
// manapun di bawah), bukan "selalu aktif" -- lihat MASTER IMAGE MOBILE CUSTOMER.md.
const dateScopeWhere = `(mimc.flag_all_date = true OR (mimc.date_start IS NOT NULL AND mimc.date_end IS NOT NULL AND CURRENT_DATE BETWEEN mimc.date_start AND mimc.date_end))`

// GetBanners: SEMUA section banner mobile customer app digabung 1 endpoint (splash, quick
// action kiri-kanan, login sheet, + 4 daftar banner) -- app butuh semuanya sekitar
// awal-buka/home, 1 round-trip lebih murah buat mobile daripada dipecah per section. PUBLIK
// (gak butuh Authorization) -- splash/login-sheet ditampilin SEBELUM member login.
//
// Header (4 slot gambar tunggal): TIAP KOLOM nyari sendiri-sendiri (2026-08-24) -- dari
// campaign yang aktif HARI INI (kena dateScopeWhere), diambil campaign PALING BARU yang kolom
// itu gak null (skip campaign yang kolomnya kosong, turun ke campaign aktif berikutnya). Bisa
// aja 4 kolom ini asalnya dari 4 campaign yang beda-beda -- lihat fetchHeaderBanners().
//
// 4 daftar banner (swipe/popup/promotion/about_us): SEBALIKNYA, diambil dari SEMUA campaign yang
// cocok (gak dibatasin ke 1 campaign kayak header) -- diurutkan campaign PALING BARU duluan
// (mimc.id DESC), baru di dalam 1 campaign yang sama urut `sequence` ASC.
//
// swipe/popup/promotion (BUKAN about_us) ditambah filter tanggal aktif PER CAMPAIGN
// (2026-08-24, dateScopeWhere -- kolomnya di header, bukan per baris banner) -- cuma nongolin
// banner dari campaign yang flag_all_date=true ATAU tanggal sekarang di antara
// date_start-date_end campaign itu.
func (h *handler) GetBanners(c fiber.Ctx) error {
	res := helpers.NewResponse()
	brandID := middleware.BrandID(c)
	ctx := c.Context()

	header, err := fetchHeaderBanners(ctx, h.db, brandID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data banner"))
	}

	swipe, err := fetchScheduledNamedBanners(ctx, h.db, "master_image_mb_cust_banner_swipe", brandID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data banner swipe"))
	}

	popup, err := fetchPopupBanners(ctx, h.db, brandID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data banner popup"))
	}

	promotion, err := fetchScheduledPromotionBanners(ctx, h.db, brandID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data banner promotion"))
	}

	// about_us TIDAK pakai fetchScheduledNamedBanners -- tabelnya emang gak punya kolom
	// flag_all_date/date_start/date_end, gak ada konsep "aktif per tanggal" buat section ini.
	aboutUs, err := fetchNamedBanners(ctx, h.db, "master_image_mb_cust_banner_about_us", brandID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data banner about us"))
	}

	return c.JSON(res.Success().SetData(bannerResponse{
		BannerSplashSrc:                 header.BannerSplashSrc,
		BannerQuickActionLeftButtonSrc:  header.BannerQuickActionLeftButtonSrc,
		BannerQuickActionLeftLink:       header.BannerQuickActionLeftLink,
		BannerQuickActionRightButtonSrc: header.BannerQuickActionRightButtonSrc,
		BannerQuickActionRightLink:      header.BannerQuickActionRightLink,
		BannerLoginSheetSrc:             header.BannerLoginSheetSrc,
		BannerSwipe:                     swipe,
		BannerPopup:                     popup,
		BannerPromotion:                 promotion,
		BannerAboutUs:                   aboutUs,
	}))
}

// fetchHeaderBanners: slot gambar tunggal header. Splash/LoginSheet TIAP KOLOM NYARI
// SENDIRI-SENDIRI (2026-08-24) -- gak ada pasangan link, independen murni: dari campaign yang
// lagi aktif HARI INI (dateScopeWhere), diambil dari yang PALING BARU (mimc.id DESC) yang kolom
// itu TIDAK NULL, turun ke campaign aktif berikutnya kalau kosong. QuickActionLeft/Right (gambar
// quick action, 2026-10-06) BEDA -- fetch BARENG link pasangannya lewat
// fetchHeaderImageWithLink(), gambar+link WAJIB dari 1 campaign yang sama (lihat komentar
// headerBanners).
func fetchHeaderBanners(ctx context.Context, db *bun.DB, brandID int) (headerBanners, error) {
	splash, err := fetchHeaderColumn(ctx, db, "banner_splash_src", brandID)
	if err != nil {
		return headerBanners{}, err
	}
	quickLeftSrc, quickLeftLink, err := fetchHeaderImageWithLink(ctx, db, "banner_quick_action_left_button_src", "banner_quick_action_left_link", brandID)
	if err != nil {
		return headerBanners{}, err
	}
	quickRightSrc, quickRightLink, err := fetchHeaderImageWithLink(ctx, db, "banner_quick_action_right_button_src", "banner_quick_action_right_link", brandID)
	if err != nil {
		return headerBanners{}, err
	}
	loginSheet, err := fetchHeaderColumn(ctx, db, "banner_login_sheet_src", brandID)
	if err != nil {
		return headerBanners{}, err
	}

	return headerBanners{
		BannerSplashSrc:                 splash,
		BannerQuickActionLeftButtonSrc:  quickLeftSrc,
		BannerQuickActionLeftLink:       quickLeftLink,
		BannerQuickActionRightButtonSrc: quickRightSrc,
		BannerQuickActionRightLink:      quickRightLink,
		BannerLoginSheetSrc:             loginSheet,
	}, nil
}

// fetchHeaderColumn: cari 1 kolom header (dari 4 nama tetap yang dipanggil fetchHeaderBanners,
// bukan input user -- aman dari SQL injection walau interpolasi string langsung) dari campaign
// PALING BARU yang aktif hari ini (dateScopeWhere) DAN kolom itu sendiri TIDAK NULL. Gak
// ketemu sama sekali -- balikin nil, BUKAN error (berarti gak ada campaign aktif yang ngisi
// kolom ini).
func fetchHeaderColumn(ctx context.Context, db *bun.DB, column string, brandID int) (*string, error) {
	var value *string
	err := db.NewRaw(`
		SELECT mimc.`+column+`
		FROM master_image_mb_cust mimc
		`+scopeJoin+`
		WHERE `+scopeWhere+` AND `+dateScopeWhere+` AND mimc.`+column+` IS NOT NULL
		ORDER BY mimc.id DESC
		LIMIT 1
	`, brandID).Scan(ctx, &value)
	if err != nil && err.Error() == "sql: no rows in result set" {
		return nil, nil
	}
	return value, err
}

// fetchHeaderImageWithLink: BEDA dari fetchHeaderColumn() -- buat pasangan gambar+link quick
// action (2026-10-06), bukan kolom gambar berdiri sendiri. Cari campaign PALING BARU yang aktif
// hari ini (dateScopeWhere) DAN kolom gambarnya TIDAK NULL (sama kriteria fetchHeaderColumn()
// buat SRC), lalu ambil DUA kolom (src+link) dari BARIS CAMPAIGN YANG SAMA itu dalam 1 query --
// link-nya WAJIB ikut campaign yang sama dengan gambarnya (disepakati eksplisit 2026-10-06), BUKAN
// dicari independen ke campaign lain kalau kosong. Kalau campaign sumber gambar itu gak isi
// link, link-nya tetep null -- bukan bug, by design (gambar+link gak boleh "ketukar campaign").
// columnSrc/columnLink WAJIB dari nama kolom tetap yang dipanggil fetchHeaderBanners, bukan
// input user -- aman dari SQL injection walau interpolasi string langsung.
func fetchHeaderImageWithLink(ctx context.Context, db *bun.DB, columnSrc string, columnLink string, brandID int) (*string, *string, error) {
	var row struct {
		Src  *string `bun:"src"`
		Link *string `bun:"link"`
	}
	err := db.NewRaw(`
		SELECT mimc.`+columnSrc+` as src, mimc.`+columnLink+` as link
		FROM master_image_mb_cust mimc
		`+scopeJoin+`
		WHERE `+scopeWhere+` AND `+dateScopeWhere+` AND mimc.`+columnSrc+` IS NOT NULL
		ORDER BY mimc.id DESC
		LIMIT 1
	`, brandID).Scan(ctx, &row)
	if err != nil && err.Error() == "sql: no rows in result set" {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return row.Src, row.Link, nil
}

// fetchNamedBanners: dipakai buat banner_about_us -- satu-satunya dari 4 daftar banner yang
// GAK ikut filter tanggal campaign (beda dari fetchScheduledNamedBanners). table WAJIB dari
// daftar tetap di banner_handler.go, bukan input user -- aman dari SQL injection walau
// interpolasi string langsung.
func fetchNamedBanners(ctx context.Context, db *bun.DB, table string, brandID int) ([]namedBanner, error) {
	list := []namedBanner{}
	err := db.NewRaw(`
		SELECT b.banner_src, b.name
		FROM `+table+` b
		JOIN master_image_mb_cust mimc ON mimc.id = b.master_image_mb_cust_id
		`+scopeJoin+`
		WHERE `+scopeWhere+`
		ORDER BY mimc.id DESC, b.sequence ASC
	`, brandID).Scan(ctx, &list)
	return list, err
}

// fetchScheduledNamedBanners: dipakai buat banner_swipe -- sama kayak fetchNamedBanners TAPI
// ditambah dateScopeWhere (filter campaign-nya, bukan filter per baris banner). table WAJIB dari
// daftar tetap di banner_handler.go. banner_promotion PISAH (fetchScheduledPromotionBanners di
// bawah, 2026-10-06) karena punya action_link yang banner_swipe gak punya.
func fetchScheduledNamedBanners(ctx context.Context, db *bun.DB, table string, brandID int) ([]namedBanner, error) {
	list := []namedBanner{}
	err := db.NewRaw(`
		SELECT b.banner_src, b.name
		FROM `+table+` b
		JOIN master_image_mb_cust mimc ON mimc.id = b.master_image_mb_cust_id
		`+scopeJoin+`
		WHERE `+scopeWhere+` AND `+dateScopeWhere+`
		ORDER BY mimc.id DESC, b.sequence ASC
	`, brandID).Scan(ctx, &list)
	return list, err
}

// fetchScheduledPromotionBanners: khusus banner_promotion (migration sudocore2 257, 2026-10-06)
// -- sama kriteria scope+tanggal kayak fetchScheduledNamedBanners, tapi ikut narik action_link
// (banner_swipe gak punya kolom ini, makanya gak bisa reuse fetchScheduledNamedBanners).
func fetchScheduledPromotionBanners(ctx context.Context, db *bun.DB, brandID int) ([]promotionBanner, error) {
	list := []promotionBanner{}
	err := db.NewRaw(`
		SELECT b.banner_src, b.name, b.action_link
		FROM master_image_mb_cust_banner_promotion b
		JOIN master_image_mb_cust mimc ON mimc.id = b.master_image_mb_cust_id
		`+scopeJoin+`
		WHERE `+scopeWhere+` AND `+dateScopeWhere+`
		ORDER BY mimc.id DESC, b.sequence ASC
	`, brandID).Scan(ctx, &list)
	return list, err
}

func fetchPopupBanners(ctx context.Context, db *bun.DB, brandID int) ([]popupBanner, error) {
	list := []popupBanner{}
	err := db.NewRaw(`
		SELECT b.banner_src, b.action_link
		FROM master_image_mb_cust_banner_popup b
		JOIN master_image_mb_cust mimc ON mimc.id = b.master_image_mb_cust_id
		`+scopeJoin+`
		WHERE `+scopeWhere+` AND `+dateScopeWhere+`
		ORDER BY mimc.id DESC, b.sequence ASC
	`, brandID).Scan(ctx, &list)
	return list, err
}
