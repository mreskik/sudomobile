package branch

import (
	"context"
	"strconv"
	"strings"
	"time"

	"sudomobile/backend/heartbeat"
	"sudomobile/backend/helpers"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type Handler interface {
	GetList(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

// weekdayNames: Go time.Weekday (Sunday=0) -> nama hari lowercase bahasa Inggris, sama
// konvensi kolom master_branch_ops_setting.day (lihat migration 081, CHECK constraint).
var weekdayNames = [7]string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

// branchListRow: hasil mentah dari DB, location_coordinate masih 1 string gabungan
// ("lat,long", lihat pecahLocationCoordinate()) -- belum di-breakdown. OpsStatus/OpenTime/
// ClosedTime dari master_branch_ops_setting HARI INI (LEFT JOIN, bisa NULL semua kalau
// belum di-setting buat hari ini).
type branchListRow struct {
	ID                 int64   `bun:"id"`
	Name               string  `bun:"name"`
	Address            string  `bun:"address"`
	BrandId            int64   `bun:"brand_id"`
	BrandName          *string `bun:"brand_name"`
	LogoBrandSrc       *string `bun:"logo_brand_src"`
	LocationCoordinate *string `bun:"location_coordinate"`
	OpsStatus          *string `bun:"ops_status"`
	OpenTime           *string `bun:"open_time"`
	ClosedTime         *string `bun:"closed_time"`
}

type branchListItem struct {
	ID                  int64    `json:"id"`
	Name                string   `json:"name"`
	Address             string   `json:"address"`
	BrandId             int64    `json:"brand_id"`
	BrandName           *string  `json:"brand_name"`
	LogoBrandSrc        *string  `json:"logo_brand_src"`
	Latitude            *float64 `json:"latitude"`
	Longitude           *float64 `json:"longitude"`
	Status              *string  `json:"status"`
	OpenTime            *string  `json:"open_time"`
	ClosedTime          *string  `json:"closed_time"`
	FlagStatusStoreOpen bool     `json:"flag_status_store_open"`
}

// GetList: daftar branch yang nerima online order lewat mobile customer app -- PUBLIK (gak
// butuh Authorization), dipakai buat misal milih lokasi pickup/pengantaran sebelum member
// login. SENGAJA gak di-scope brand_id (2026-08-24, konfirmasi eksplisit) -- beda dari
// GetBanners(), semua branch yang online-order-nya aktif ditampilin gak peduli brand.
//
// Filter: master_branch_setting.flag_online_service_mobile_customer = true (branch-nya
// diaktifin admin buat nerima order online mobile) DAN master_branch.status = '1' (branch-nya
// sendiri masih aktif/gak ditutup -- branch nonaktif gak boleh muncul walau flag setting-nya
// kebetulan true).
//
// logo_brand_src -- LOGO BRAND (master_brand.logo_path), BUKAN logo_header_src milik branch
// itu sendiri (2026-08-24, dikoreksi) -- LEFT JOIN (bukan JOIN) biar branch tetep muncul kalau
// somehow brand-nya gak ketemu, logo-nya null aja.
//
// status/open_time/closed_time (2026-08-24) -- jam operasional HARI INI dari
// master_branch_ops_setting, sama logic kayak DayShiftServices::GetOperationalHoursToday() di
// POS (posv1-laravel/app/Services/DayShiftServices.php). Ops setting belum di-setting buat hari
// ini -- balikin null/false semua, BUKAN error (info tambahan doang, branch tetep muncul di list).
//
// flag_status_store_open (2026-08-27, UPDATE) -- SEKARANG gabungan 2 syarat: jam operasional
// (IsStoreOpenNow(), di atas) DAN branch-nya online (heartbeat.IsOnline(), lihat SEND
// HEARTBEAT.md di posv1-laravel) -- dua-duanya harus true. Sebelumnya field ini MURNI info jam
// operasional doang; sekarang jadi indikasi "beneran bisa dipesan sekarang", walau gerbang
// SEBENARNYA yang nolak order tetap di Create() (order/order_create_handler.go, heartbeat.IsOnline()
// dicek ULANG di situ) -- field ini display doang, BUKAN otoritas final (client jangan
// nge-skip validasi di server dengan asumsi flag ini akurat 100% saat order beneran disubmit,
// ada jeda waktu antara nge-list branch & submit order).
func (h *handler) GetList(c fiber.Ctx) error {
	res := helpers.NewResponse()

	today := weekdayNames[time.Now().Weekday()]

	rows := []branchListRow{}
	err := h.db.NewRaw(`
		SELECT
			mb.id, mb.name, mb.address,
			mb.brand_id, mbr.name AS brand_name, mbr.logo_path AS logo_brand_src,
			mb.location_coordinate,
			mbos.status AS ops_status, mbos.open_time, mbos.closed_time
		FROM master_branch mb
		JOIN master_branch_setting mbs ON mbs.branch_id = mb.id
		LEFT JOIN master_brand mbr ON mbr.id = mb.brand_id
		LEFT JOIN master_branch_ops_setting mbos ON mbos.branch_id = mb.id AND mbos.day = ?
		WHERE mbs.flag_online_service_mobile_customer = true AND mb.status = '1'
		ORDER BY mb.name ASC
	`, today).Scan(c.Context(), &rows)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data branch"))
	}

	now := time.Now().Format("15:04:05")

	list := make([]branchListItem, 0, len(rows))
	for _, row := range rows {
		lat, lng := pecahLocationCoordinate(row.LocationCoordinate)
		list = append(list, branchListItem{
			ID:                  row.ID,
			Name:                row.Name,
			Address:             row.Address,
			BrandId:             row.BrandId,
			BrandName:           row.BrandName,
			LogoBrandSrc:        row.LogoBrandSrc,
			Latitude:            lat,
			Longitude:           lng,
			Status:              row.OpsStatus,
			OpenTime:            row.OpenTime,
			ClosedTime:          row.ClosedTime,
			FlagStatusStoreOpen: IsStoreOpenNow(row.OpsStatus, row.OpenTime, row.ClosedTime, now) && heartbeat.IsOnline(c.Context(), h.db, int(row.ID)),
		})
	}

	return c.JSON(res.Success().SetData(list))
}

// IsStoreOpenNow: replika DayShiftServices::GetOperationalHoursToday() (POS) -- always_open
// selalu true, open dicek terhadap jam sekarang (string compare "HH:MM:SS", SAMA kayak PHP-nya,
// jadi rentang yang nyebrang tengah malam misal 18:00-02:00 juga SAMA-SAMA belum dihandle
// bener di sini, konsisten sama keterbatasan yang udah ada -- bukan bug baru), closed/null
// selalu false. Exported (2026-08-27) -- dipakai lintas modul, lihat IsOpenNow() di bawah.
func IsStoreOpenNow(status, openTime, closedTime *string, now string) bool {
	if status == nil {
		return false
	}
	switch *status {
	case "always_open":
		return true
	case "open":
		if openTime == nil || closedTime == nil {
			return false
		}
		return now >= *openTime && now <= *closedTime
	default: // "closed"
		return false
	}
}

// IsOpenNow: versi self-contained IsStoreOpenNow() -- query sendiri jam operasional HARI INI
// buat 1 branch, dipakai modul LAIN yang belum punya baris master_branch_ops_setting di tangan
// (beda dari GetList() di atas, yang udah JOIN sekaligus buat banyak branch dalam 1 query, gak
// cocok dipanggil per-branch di sini -- bakal N+1 kalau dipaksa reuse). Dipakai
// order/order_create_handler.go buat barrier keras Create() (2026-08-27) -- lihat GET VISIT
// PURPOSE DETAIL.md / KIOSK BRANCH VISIT PURPOSE DETAIL.md buat konteks kenapa jam operasional
// akhirnya jadi gerbang keras juga (sebelumnya cuma info tampilan di List, sekarang dobel
// dipakai). Error DB / baris gak ketemu -> false (fail-safe, sama semangatnya kayak
// heartbeat.IsOnline()).
func IsOpenNow(ctx context.Context, db *bun.DB, branchID int) bool {
	today := weekdayNames[time.Now().Weekday()]

	var status, openTime, closedTime *string
	err := db.NewRaw(`
		SELECT status, open_time, closed_time FROM master_branch_ops_setting
		WHERE branch_id = ? AND day = ?
	`, branchID, today).Scan(ctx, &status, &openTime, &closedTime)
	if err != nil {
		return false
	}

	return IsStoreOpenNow(status, openTime, closedTime, time.Now().Format("15:04:05"))
}

// pecahLocationCoordinate: location_coordinate di DB isinya 1 string "latitude,longitude"
// (lihat placeholder form ERP: "-6.1800448,106.9481984" -- Jakarta, lat duluan baru long).
// Dipecah jadi 2 field terpisah biar mobile app gak perlu parsing string sendiri. Data-nya
// gak selalu valid (ada baris test isinya "tes" dkk) -- kalau formatnya bukan "angka,angka"
// persis (bukan 2 bagian, atau salah satu gagal di-parse float), balikin nil buat KEDUANYA
// (bukan cuma yang gagal), biar konsisten "ada koordinat" vs "gak ada", gak pernah setengah.
func pecahLocationCoordinate(raw *string) (*float64, *float64) {
	if raw == nil {
		return nil, nil
	}

	parts := strings.Split(*raw, ",")
	if len(parts) != 2 {
		return nil, nil
	}

	lat, errLat := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lng, errLng := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if errLat != nil || errLng != nil {
		return nil, nil
	}

	return &lat, &lng
}
