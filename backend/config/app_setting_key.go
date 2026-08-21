package config

import (
	"encoding/base64"
	"log"
	"os"
)

// AppSettingKey: 32 byte AES-256, dipakai dekripsi header X-App-Setting. Statis di .env buat
// sekarang (APP_SETTING_KEY, base64) -- belum per-device/provisioning, itu dibahas belakangan.
var AppSettingKey []byte

func InitAppSettingKey() {
	raw := os.Getenv("APP_SETTING_KEY")
	if raw == "" {
		log.Fatal("APP_SETTING_KEY belum di-set di .env")
	}

	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		log.Fatal("APP_SETTING_KEY bukan base64 valid : ", err)
	}
	if len(key) != 32 {
		log.Fatalf("APP_SETTING_KEY harus 32 byte (AES-256) setelah di-decode, sekarang %d byte", len(key))
	}

	AppSettingKey = key
	log.Println("App setting key loaded !")
}
