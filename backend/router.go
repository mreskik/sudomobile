package backend

import (
	"sudomobile/backend/config"
	"sudomobile/backend/middleware"
	"sudomobile/backend/modules/account"
	"sudomobile/backend/modules/auth"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
)

// RegisterRoutes: semua endpoint customer mobile app nempel di sini. Beda dari APIANDORDER
// (/pos/*, BranchTokenAuth per-device), semua route di sini scoped ke 1 customer
// (master_member), auth-nya middleware.Auth (session token hasil Register/LoginOTP/LoginPin).
func RegisterRoutes(app *fiber.App) {
	// AppSetting dipasang PALING LUAR -- wajib di semua route termasuk /api/auth/*
	// (belum login pun app-nya tetep harus ngirim header X-App-Setting, itu app config bukan
	// identitas customer). Lihat middleware/app_setting.go.
	root := app.Group("/api", middleware.AppSetting(config.DB, config.AppSettingKey))

	authHandler := auth.NewHandler(config.DB)
	authRouter := root.Group("/auth")
	authRouter.Post("/check_number", authHandler.CheckNumber)
	authRouter.Post("/request_otp", authHandler.RequestOTP)
	authRouter.Post("/register", authHandler.Register)
	authRouter.Post("/login_otp", authHandler.LoginOTP)
	authRouter.Post("/login_pin", authHandler.LoginPin)
	authRouter.Post("/pin/reset", authHandler.ResetPin)

	// PROTECTED -- wajib session token (middleware.Auth), member_id diambil dari situ.
	protectedAuthRouter := root.Group("/auth", middleware.Auth(config.DB))
	protectedAuthRouter.Post("/pin/create", authHandler.CreatePin)
	protectedAuthRouter.Post("/pin/change", authHandler.ChangePin)

	// PROTECTED -- profil akun customer yang lagi login.
	accountHandler := account.NewHandler(config.DB)
	accountRouter := root.Group("/account", middleware.Auth(config.DB))
	accountRouter.Get("/me", accountHandler.Me)
	accountRouter.Get("/balance", accountHandler.Balance)
	accountRouter.Get("/balance/history", accountHandler.BalanceHistory)
	accountRouter.Get("/point", accountHandler.Point)
	accountRouter.Get("/point/history", accountHandler.PointHistory)
	accountRouter.Get("/tier-list", accountHandler.TierList)
	accountRouter.Post("/photo", accountHandler.UpdatePhoto)

	// Static file serving buat foto profil (dan file upload lain di sudomobile ke depannya) --
	// path storage-nya harus match photoStorageRoot di modules/account/photo_handler.go.
	app.Get("/storage/*", static.New("./storage"))
}
