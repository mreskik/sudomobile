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

// headerBanners: dibangun manual dari 4 hasil fetchHeaderColumn() terpisah (masing-masing bisa
// dari campaign berbeda) -- bukan hasil 1 query yang di-scan langsung, jadi gak ada bun tag.
type headerBanners struct {
	BannerSplashSrc                 *string
	BannerQuickActionLeftButtonSrc  *string
	BannerQuickActionRightButtonSrc *string
	BannerLoginSheetSrc             *string
}

// Sequence SENGAJA gak ikut di struct/response -- cuma dipakai buat ORDER BY di query
// (fetchNamedBanners/fetchPopupBanners), datanya udah kekirim urut, frontend gak perlu tau
// angka mentahnya.
type namedBanner struct {
	BannerSrc string `json:"banner_src" bun:"banner_src"`
	Name      string `json:"name" bun:"name"`
}

type popupBanner struct {
	BannerSrc  string  `json:"banner_src" bun:"banner_src"`
	ActionLink *string `json:"action_link" bun:"action_link"`
}

type bannerResponse struct {
	BannerSplashSrc                 *string       `json:"banner_splash_src"`
	BannerQuickActionLeftButtonSrc  *string       `json:"banner_quick_action_left_button_src"`
	BannerQuickActionRightButtonSrc *string       `json:"banner_quick_action_right_button_src"`
	BannerLoginSheetSrc             *string       `json:"banner_login_sheet_src"`
	BannerSwipe                     []namedBanner `json:"banner_swipe"`
	BannerPopup                     []popupBanner `json:"banner_popup"`
	BannerPromotion                 []namedBanner `json:"banner_promotion"`
	BannerAboutUs                   []namedBanner `json:"banner_about_us"`
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

	promotion, err := fetchScheduledNamedBanners(ctx, h.db, "master_image_mb_cust_banner_promotion", brandID)
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
		BannerQuickActionRightButtonSrc: header.BannerQuickActionRightButtonSrc,
		BannerLoginSheetSrc:             header.BannerLoginSheetSrc,
		BannerSwipe:                     swipe,
		BannerPopup:                     popup,
		BannerPromotion:                 promotion,
		BannerAboutUs:                   aboutUs,
	}))
}

// fetchHeaderBanners: 4 slot gambar tunggal, TIAP KOLOM NYARI SENDIRI-SENDIRI (2026-08-24) --
// gak lagi "1 campaign buat 4 kolom sekaligus". Tiap kolom independen: dari campaign yang lagi
// aktif HARI INI (kena dateScopeWhere -- beda dari sebelumnya yang ngabaikan tanggal sama
// sekali), diambil dari yang PALING BARU (mimc.id DESC) yang kolom itu TIDAK NULL. Kalau
// campaign terbaru yang aktif kolomnya kosong, turun ke campaign aktif berikutnya yang lebih
// lama, dst -- bukan langsung nyerah jadi null. Efeknya 4 kolom ini bisa aja asalnya dari 4
// campaign yang beda-beda.
func fetchHeaderBanners(ctx context.Context, db *bun.DB, brandID int) (headerBanners, error) {
	splash, err := fetchHeaderColumn(ctx, db, "banner_splash_src", brandID)
	if err != nil {
		return headerBanners{}, err
	}
	quickLeft, err := fetchHeaderColumn(ctx, db, "banner_quick_action_left_button_src", brandID)
	if err != nil {
		return headerBanners{}, err
	}
	quickRight, err := fetchHeaderColumn(ctx, db, "banner_quick_action_right_button_src", brandID)
	if err != nil {
		return headerBanners{}, err
	}
	loginSheet, err := fetchHeaderColumn(ctx, db, "banner_login_sheet_src", brandID)
	if err != nil {
		return headerBanners{}, err
	}

	return headerBanners{
		BannerSplashSrc:                 splash,
		BannerQuickActionLeftButtonSrc:  quickLeft,
		BannerQuickActionRightButtonSrc: quickRight,
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

// fetchScheduledNamedBanners: dipakai buat banner_swipe/banner_promotion -- sama kayak
// fetchNamedBanners TAPI ditambah dateScopeWhere (filter campaign-nya, bukan filter per baris
// banner). table WAJIB dari daftar tetap di banner_handler.go.
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
