package main

import (
	"log"
	"os"

	"sudomobile/backend"
	"sudomobile/backend/config"
	"sudomobile/backend/modules/orderexpiry"

	"github.com/gofiber/fiber/v3"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Gagal load .env, lanjut pakai env yang udah ke-set : ", err)
	}

	config.InitDB()
	config.InitAppSettingKey()
	config.InitStoragePath()
	config.InitPaymentGatewayEndpoint()

	// Background job -- lihat DOKUMENTASI BACKGROUND JOB/POLA UMUM.md.
	go orderexpiry.RunLoop()

	app := fiber.New()

	backend.RegisterRoutes(app)

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "96"
	}

	log.Fatal(app.Listen("0.0.0.0:" + port))
}
