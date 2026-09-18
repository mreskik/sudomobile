package branch

import (
	"time"

	"sudomobile/backend/heartbeat"
	"sudomobile/backend/helpers"
	"sudomobile/backend/modules/qrorder"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

// qrBranchListRow/qrBranchListItem: versi QR Order dari branchListRow/branchListItem (di atas) --
// TAMBAH `Code` (QR Order navigasi pakai code, bukan id, buat request berikutnya) & di-scope 1
// company (bukan semua branch lintas company kayak GetList() member app). Struct TERPISAH
// (bukan nambahin Code ke branchListRow) -- pola yang udah konsisten dipakai QR Order sepanjang
// sesi 2026-09-17 (qrOrderDetailHeader vs orderDetailHeader, dst): field set beda dikit, query
// beda (WHERE company_id tambahan), gak worth digabung jadi 1 struct dgn field kadang dipakai
// kadang enggak.
type qrBranchListRow struct {
	ID                 int64   `bun:"id"`
	Code               string  `bun:"code"`
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

type qrBranchListItem struct {
	ID                  int64    `json:"id"`
	Code                string   `json:"code"`
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

// QRHandler: interface TERPISAH dari Handler (member app) -- pola sama kayak QRHandler di
// package order/paymentmethod/visitpurpose (QR Order publik total, gak ada middleware.AppSetting).
type QRHandler interface {
	GetList(c fiber.Ctx) error
}

type qrHandler struct {
	db *bun.DB
}

func NewQRHandler(db *bun.DB) QRHandler {
	return &qrHandler{db: db}
}

// GetList: GET /qr-order/branch-list?db_code=&company_code= -- versi QR Order dari GetList()
// (member app) di atas, DI-SCOPE 1 company (`qrorder.ResolveCompany()`, cuma 2 kode -- belum
// butuh branch_code/visit_purpose_code, endpoint ini justru buat NEMUIN branch_code-nya).
// Filter & logic PERSIS SAMA (flag_online_service_mobile_customer + status aktif, jam operasional
// hari ini, heartbeat) -- ditulis ulang (bukan diekstrak) karena kolom company_id di WHERE clause
// & Code di SELECT bikin query-nya beda dari punya member app, sama pertimbangan kayak struct di
// atas. Lihat DOKUMENTASI API/QR ORDER/ORDER/GET BRANCH LIST.md.
func (h *qrHandler) GetList(c fiber.Ctx) error {
	res := helpers.NewResponse()
	ctx := c.Context()

	companyCtx, errMsg, err := qrorder.ResolveCompany(ctx, h.db, c.Query("db_code"), c.Query("company_code"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal validasi identitas request"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	today := weekdayNames[time.Now().Weekday()]

	rows := []qrBranchListRow{}
	err = h.db.NewRaw(`
		SELECT
			mb.id, mb.code, mb.name, mb.address,
			mb.brand_id, mbr.name AS brand_name, mbr.logo_path AS logo_brand_src,
			mb.location_coordinate,
			mbos.status AS ops_status, mbos.open_time, mbos.closed_time
		FROM master_branch mb
		JOIN master_branch_setting mbs ON mbs.branch_id = mb.id
		LEFT JOIN master_brand mbr ON mbr.id = mb.brand_id
		LEFT JOIN master_branch_ops_setting mbos ON mbos.branch_id = mb.id AND mbos.day = ?
		WHERE mbs.flag_online_service_mobile_customer = true AND mb.status = '1' AND mb.company_id = ?
		ORDER BY mb.name ASC
	`, today, companyCtx.CompanyID).Scan(ctx, &rows)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data branch"))
	}

	now := time.Now().Format("15:04:05")

	list := make([]qrBranchListItem, 0, len(rows))
	for _, row := range rows {
		lat, lng := pecahLocationCoordinate(row.LocationCoordinate)
		list = append(list, qrBranchListItem{
			ID:                  row.ID,
			Code:                row.Code,
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
			FlagStatusStoreOpen: IsStoreOpenNow(row.OpsStatus, row.OpenTime, row.ClosedTime, now) && heartbeat.IsOnline(ctx, h.db, int(row.ID)),
		})
	}

	return c.JSON(res.Success().SetData(list))
}
