package backend

import (
	"sudomobile/backend/config"
	"sudomobile/backend/middleware"
	"sudomobile/backend/modules/account"
	"sudomobile/backend/modules/auth"
	"sudomobile/backend/modules/banner"
	"sudomobile/backend/modules/bestseller"
	"sudomobile/backend/modules/branch"
	"sudomobile/backend/modules/order"
	"sudomobile/backend/modules/paymentmethod"
	"sudomobile/backend/modules/promo"
	"sudomobile/backend/modules/visitpurpose"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/static"
)

// RegisterRoutes: semua endpoint customer mobile app nempel di sini. Beda dari APIANDORDER
// (/pos/*, BranchTokenAuth per-device), semua route di sini scoped ke 1 customer
// (master_member), auth-nya middleware.Auth (session token hasil Register/LoginOTP/LoginPin).
func RegisterRoutes(app *fiber.App) {
	// CORS dipasang GLOBAL di app (bukan di-scope ke grup /api doang) -- nyakup SEMUA route
	// termasuk /storage/* (static file foto profil) juga, gak cuma /api/*. Dipasang paling
	// awal sebelum apapun -- preflight request (OPTIONS) dari browser gak bakal nyertain header
	// custom (X-App-Setting/Authorization), jadi CORS-nya harus udah jawab duluan sebelum
	// request itu sempat ditolak middleware lain manapun. Config disamain sama konvensi
	// sudocore2 (backend/routes/backend_routes.go): buka semua origin, gak pakai credentials
	// (cookie/session browser) -- auth-nya lewat Bearer token di header, bukan cookie, jadi
	// AllowCredentials gak relevan di sini.
	//
	// AllowOrigins di Fiber v3 formatnya []string (beda dari v2 yang string tunggal
	// "*" dipisah koma) -- "*" di elemen manapun berarti semua origin diizinkan.
	app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowCredentials: false,
	}))

	// AppSetting wajib di semua route termasuk /api/auth/* (belum login pun app-nya tetep
	// harus ngirim header X-App-Setting, itu app config bukan identitas customer). Lihat
	// middleware/app_setting.go.
	root := app.Group("/api", middleware.AppSetting(config.DB, config.AppSettingKey))

	authHandler := auth.NewHandler(config.DB)
	authRouter := root.Group("/auth")
	authRouter.Post("/check_number", authHandler.CheckNumber)
	authRouter.Post("/request_otp", authHandler.RequestOTP)
	authRouter.Post("/register", authHandler.Register)
	authRouter.Post("/login_otp", authHandler.LoginOTP)
	authRouter.Post("/login_pin", authHandler.LoginPin)
	authRouter.Post("/pin/reset", authHandler.ResetPin)

	// PUBLIK -- splash/login-sheet ditampilin sebelum member login, jadi endpoint ini gak
	// boleh Protected. Scoping brand dari X-App-Setting (middleware.BrandID), bukan
	// Authorization.
	bannerHandler := banner.NewHandler(config.DB)
	root.Get("/banner", bannerHandler.GetBanners)

	// PUBLIK -- daftar branch yang nerima online order lewat mobile, dipakai buat misal milih
	// lokasi pickup sebelum login. Gak di-scope brand_id (2026-08-24, konfirmasi eksplisit).
	branchHandler := branch.NewHandler(config.DB)
	root.Get("/branch", branchHandler.GetList)

	// PUBLIK -- daftar visit purpose yang dibolehin muncul di mobile customer app buat 1
	// branch (branch_id eksplisit di URL, konfirmasi 2026-08-24) -- filter
	// flag_mobile_customer, mirror Kiosk POS tapi flag beda & branch_id gak implisit.
	visitPurposeHandler := visitpurpose.NewHandler(config.DB)
	root.Get("/branch/:branch_id/visit-purpose", visitPurposeHandler.GetList)
	root.Get("/branch/:branch_id/visit-purpose/:visit_purpose_id", visitPurposeHandler.GetDetail)

	// PUBLIK -- daftar payment method yang bisa dipakai buat 1 branch+visit_purpose (gateway-only,
	// konfirmasi 2026-08-24). Nested di bawah visit-purpose karena scoping-nya sama.
	paymentMethodHandler := paymentmethod.NewHandler(config.DB)
	root.Get("/branch/:branch_id/visit-purpose/:visit_purpose_id/payment-method", paymentMethodHandler.GetList)

	// PUBLIK -- best seller, sumber data mb_order/mb_order_detail DOANG (bukan gabung POS,
	// 2026-08-25 konfirmasi eksplisit), 30 hari terakhir, cuma order status='paid'. 3 endpoint
	// beda scope -- makin sempit scope-nya, makin lengkap datanya (harga cuma ada di scope
	// branch+visit_purpose, karena menu_template_id baru deterministik di situ). Lihat
	// DOKUMENTASI API/MENU/GET BEST SELLER.md.
	bestSellerHandler := bestseller.NewHandler(config.DB)
	root.Get("/menu/best-seller", bestSellerHandler.GetGlobal)
	root.Get("/branch/:branch_id/best-seller", bestSellerHandler.GetByBranch)
	root.Get("/branch/:branch_id/visit-purpose/:visit_purpose_id/best-seller", bestSellerHandler.GetByVisitPurpose)

	// PROTECTED -- daftar promo yang eligible buat 1 branch+visit_purpose+member yang login
	// (filter member_type butuh identitas member). Nested di bawah visit-purpose, sama alasan
	// scoping kayak payment-method. Lihat DOKUMENTASI API/ORDER/KETENTUAN PROMO.md.
	promoHandler := promo.NewHandler(config.DB)
	promoRouter := root.Group("/branch/:branch_id/visit-purpose/:visit_purpose_id/promo", middleware.Auth(config.DB))
	promoRouter.Get("", promoHandler.GetList)

	// PROTECTED -- wajib session token (middleware.Auth), member_id diambil dari situ.
	protectedAuthRouter := root.Group("/auth", middleware.Auth(config.DB))
	protectedAuthRouter.Post("/pin/create", authHandler.CreatePin)
	protectedAuthRouter.Post("/pin/change", authHandler.ChangePin)
	protectedAuthRouter.Post("/logout", authHandler.Logout)

	// PROTECTED (2026-08-24, digeser dari publik) -- preview breakdown harga/pajak SEBELUM
	// order beneran disubmit, baca-only (gak insert apa pun ke mb_order*). Wajib login karena
	// validasi promo butuh identitas member (member_type_id buat master_promo_type_members,
	// saldo poin buat min_point_amount). Body sama persis kayak yang dipakai POST
	// /api/order/create-order.
	orderHandler := order.NewHandler(config.DB)
	orderRouter := root.Group("/order", middleware.Auth(config.DB))
	orderRouter.Post("/calculate", orderHandler.Calculate)
	// PROTECTED -- save order beneran (insert mb_order*) + trigger payment gateway (service
	// `payment`, dev/payment/) dalam 1 call. Lihat DOKUMENTASI API/ORDER/CREATE ORDER.md.
	orderRouter.Post("/create-order", orderHandler.Create)
	// PROTECTED -- polling status pembayaran (live-check ke service `payment`), finalisasi
	// mb_order_payment pas settlement, sinkronin mb_order.status pas expired. Lihat
	// DOKUMENTASI API/ORDER/PAYMENT STATUS.md.
	orderRouter.Get("/:order_number/payment-status", orderHandler.CheckPaymentStatus)
	// PROTECTED -- batalin order SEBELUM bayar (race-guard aware, lihat DOKUMENTASI
	// API/ORDER/CANCEL ORDER.md).
	orderRouter.Post("/:order_number/cancel", orderHandler.CancelOrder)
	// PROTECTED -- riwayat order milik member yang login. Lihat DOKUMENTASI
	// API/ORDER/ORDER HISTORY.md.
	orderRouter.Get("/history", orderHandler.GetHistory)
	// PROTECTED -- detail lengkap 1 order (breakdown item + QR ulang kalau masih pending).
	// Lihat DOKUMENTASI API/ORDER/ORDER DETAIL.md.
	orderRouter.Get("/:order_number", orderHandler.GetDetail)

	// PROTECTED -- profil akun customer yang lagi login.
	accountHandler := account.NewHandler(config.DB)
	accountRouter := root.Group("/account", middleware.Auth(config.DB))
	accountRouter.Get("/me", accountHandler.Me)
	accountRouter.Put("/me", accountHandler.UpdateMe)
	accountRouter.Get("/balance", accountHandler.Balance)
	accountRouter.Get("/balance/history", accountHandler.BalanceHistory)
	accountRouter.Get("/point", accountHandler.Point)
	accountRouter.Get("/point/history", accountHandler.PointHistory)
	accountRouter.Get("/tier-list", accountHandler.TierList)
	accountRouter.Post("/photo", accountHandler.UpdatePhoto)
	accountRouter.Get("/tier-spending", accountHandler.TierSpending)

	// Static file serving -- root-nya config.StoragePath (default "./storage" folder sendiri,
	// atau di-mount ke storage sudocore2 langsung lewat env STORAGE_PATH, lihat
	// backend/config/storage.go). Path-nya harus tetep match savePhoto() di
	// modules/account/photo_handler.go.
	app.Get("/storage/*", static.New(config.StoragePath))
}
