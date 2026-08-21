package config

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/extra/bundebug"
)

var DB *bun.DB

// InitDB: konek langsung ke Postgres ERP (db_sudocore_dev) yang sama dipakai sudocore2 --
// sudomobile query langsung ke situ (master_member/master_promo/dst), sama pola kayak
// APIANDORDER, bukan proxy HTTP ke sudocore2.
func InitDB() {
	dsn := "postgres://" + os.Getenv("DB_USER") + ":" + os.Getenv("DB_PASS") + "@" + os.Getenv("DB_HOST") + ":" + os.Getenv("DB_PORT") + "/" + os.Getenv("DB_NAME") + "?sslmode=disable"

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal("Error open koneksi : ", err)
	}

	if os.Getenv("APP_ENV") == "development" {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(5)
	} else {
		sqlDB.SetMaxOpenConns(20)
		sqlDB.SetMaxIdleConns(10)
	}
	sqlDB.SetConnMaxLifetime(time.Hour)

	DB = bun.NewDB(sqlDB, pgdialect.New())

	if os.Getenv("APP_ENV") == "development" {
		DB.AddQueryHook(bundebug.NewQueryHook(bundebug.WithVerbose(true)))
	}

	log.Println("Database Connected !")
}
