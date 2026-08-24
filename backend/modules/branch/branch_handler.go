package branch

import (
	"strconv"
	"strings"
	"time"

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
// status/open_time/closed_time/flag_status_store_open (2026-08-24) -- jam operasional HARI INI
// dari master_branch_ops_setting, sama logic kayak DayShiftServices::GetOperationalHoursToday()
// di POS (posv1-laravel/app/Services/DayShiftServices.php) -- dipilih pola itu (bukan
// GetKioskDayStatus() yang gabung status dayshift) karena ini murni info tampilan buat
// customer milih branch, bukan gerbang boleh/gak-nya order (gak ada konsep dayshift di
// sudomobile). Ops setting belum di-setting buat hari ini -- balikin null/false semua, BUKAN
// error (info tambahan doang, branch tetep muncul di list).
func (h *handler) GetList(c fiber.Ctx) error {
	res := helpers.NewResponse()

	today := weekdayNames[time.Now().Weekday()]

	rows := []branchListRow{}
	err := h.db.NewRaw(`
		SELECT
			mb.id, mb.name, mb.address,
			mbr.name AS brand_name, mbr.logo_path AS logo_brand_src,
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
			BrandName:           row.BrandName,
			LogoBrandSrc:        row.LogoBrandSrc,
			Latitude:            lat,
			Longitude:           lng,
			Status:              row.OpsStatus,
			OpenTime:            row.OpenTime,
			ClosedTime:          row.ClosedTime,
			FlagStatusStoreOpen: isStoreOpenNow(row.OpsStatus, row.OpenTime, row.ClosedTime, now),
		})
	}

	return c.JSON(res.Success().SetData(list))
}

// isStoreOpenNow: replika DayShiftServices::GetOperationalHoursToday() (POS) -- always_open
// selalu true, open dicek terhadap jam sekarang (string compare "HH:MM:SS", SAMA kayak PHP-nya,
// jadi rentang yang nyebrang tengah malam misal 18:00-02:00 juga SAMA-SAMA belum dihandle
// bener di sini, konsisten sama keterbatasan yang udah ada -- bukan bug baru), closed/null
// selalu false.
func isStoreOpenNow(status, openTime, closedTime *string, now string) bool {
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
