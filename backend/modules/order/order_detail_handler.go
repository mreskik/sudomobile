package order

import (
	"context"
	"database/sql"
	"errors"

	"sudomobile/backend/helpers"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

// MemberID *int64 (2026-09-17, migration sudocore2 208) -- mb_order.member_id sekarang NULLABLE
// (order tamu, baik QR Order maupun member app yang gak login -- lihat catatan 2026-09-22 di
// router.go). Pointer WAJIB biar gak Scan error. GetDetail() SENGAJA TIDAK cek kepemilikan lagi
// (dulu lewat isMemberOwner()) -- order_number itu sendiri jadi kunci akses, siapa pun yang
// tau/pegang nomornya berhak liat detailnya, endpoint ini publik (gak wajib Authorization).
type orderDetailHeader struct {
	MemberID            *int64  `bun:"member_id"`
	OrderNumber         string  `bun:"order_number"`
	Status              string  `bun:"status"`
	CreatedAt           string  `bun:"created_at"`
	SubTotal            string  `bun:"sub_total"`
	TotalDiscount       string  `bun:"total_discount"`
	TotalTax            string  `bun:"total_tax"`
	TotalBilling        string  `bun:"total_billing"`
	FlagInclusiveTax    bool    `bun:"flag_inclusive_tax"`
	CustomerPhoneNumber *string `bun:"customer_phone_number"`
	BranchID            int64   `bun:"branch_id"`
	BranchName          *string `bun:"branch_name"`
	VisitPurposeID      int64   `bun:"visit_purpose_id"`
	VisitPurposeName    *string `bun:"visit_purpose_name"`
}

type orderDetailPackageItem struct {
	MenuPackageID int64   `json:"menu_package_id" bun:"menu_package_id"`
	ItemID        int64   `json:"item_id" bun:"menu_id"`
	ItemName      *string `json:"item_name" bun:"item_name"`
	Qty           int64   `json:"qty" bun:"qty"`
	Price         string  `json:"price" bun:"price"`
	TaxType       *string `json:"tax_type" bun:"tax_type"`
	TaxRate       *string `json:"tax_rate" bun:"tax_rate"`
	DPP           *string `json:"dpp" bun:"dpp"`
	NetDPP        *string `json:"net_dpp" bun:"net_dpp"`
	TaxAmount     *string `json:"tax_amount" bun:"tax_amount"`
	Total         string  `json:"total" bun:"total"`
}

type orderDetailPackageRow struct {
	ParentULID string `bun:"mb_order_detail_ulid"`
	orderDetailPackageItem
}

type orderDetailItem struct {
	ULID            string                   `json:"-" bun:"ulid"`
	MenuID          int64                    `json:"menu_id" bun:"menu_id"`
	ItemName        *string                  `json:"item_name" bun:"item_name"`
	Qty             int64                    `json:"qty" bun:"qty"`
	Notes           *string                  `json:"notes" bun:"notes"`
	Price           string                   `json:"price" bun:"price"`
	TaxType         *string                  `json:"tax_type" bun:"tax_type"`
	TaxRate         *string                  `json:"tax_rate" bun:"tax_rate"`
	DPP             *string                  `json:"dpp" bun:"dpp"`
	NetDPP          *string                  `json:"net_dpp" bun:"net_dpp"`
	TaxAmount       *string                  `json:"tax_amount" bun:"tax_amount"`
	Total           string                   `json:"total" bun:"total"`
	PromoID         *int64                   `json:"promo_id" bun:"promo_id"`
	DiscountPercent string                   `json:"discount_percent" bun:"discount_percent"`
	DiscountAmount  string                   `json:"discount_amount" bun:"discount_amount"`
	Packages        []orderDetailPackageItem `json:"packages"`
}

type orderDetailPayment struct {
	Status            string  `json:"status"`
	PaymentMethodID   *int64  `json:"payment_method_id"`
	PaymentMethodName *string `json:"payment_method_name"`
	VendorQRString    *string `json:"vendor_qr_string"`
	VendorQRURL       *string `json:"vendor_qr_url"`
	ExpiredAt         *string `json:"expired_at"`
}

type orderDetailResult struct {
	OrderNumber         string             `json:"order_number"`
	Status              string             `json:"status"`
	CreatedAt           string             `json:"created_at"`
	BranchID            int64              `json:"branch_id"`
	BranchName          *string            `json:"branch_name"`
	VisitPurposeID      int64              `json:"visit_purpose_id"`
	VisitPurposeName    *string            `json:"visit_purpose_name"`
	CustomerPhoneNumber *string            `json:"customer_phone_number"`
	FlagInclusiveTax    bool               `json:"flag_inclusive_tax"`
	SubTotal            string             `json:"sub_total"`
	TotalDiscount       string             `json:"total_discount"`
	TotalTax            string             `json:"total_tax"`
	TotalBilling        string             `json:"total_billing"`
	Items               []orderDetailItem  `json:"items"`
	Payment             orderDetailPayment `json:"payment"`
}

// GetDetail: GET /api/order/:order_number -- detail LENGKAP 1 order (breakdown item + status
// pembayaran ter-update). Kebutuhannya (dikonfirmasi 2026-08-25): (1) struk digital -- customer
// liat rincian per-item/pajak/diskon dari order lampau, (2) NAMPILIN ULANG QR buat order yang
// masih `pending` -- vendor_qr_string/url CUMA dibalikin sekali di response create-order, gak
// pernah disimpen lokal (cuma expired_at yang kesimpen di mb_order_payment_request). Kalau
// customer nutup app sebelum sempat scan, QR-nya "ilang" dari sisi sudomobile -- tapi service
// `payment` SENDIRI tetap nyimpen vendor_qr_string/url di tabel payment_gateway-nya, jadi
// tinggal di-live-fetch ulang lewat SyncPaymentStatus() (fungsi SAMA yang dipakai
// CheckPaymentStatus()) -- BUKAN minta QR baru (retry), cuma nampilin ulang yang lama.
func (h *handler) GetDetail(c fiber.Ctx) error {
	res := helpers.NewResponse()
	orderNumber := c.Params("order_number")

	ctx := c.Context()

	var header orderDetailHeader
	err := h.db.NewRaw(`
		SELECT mo.member_id, mo.order_number, mo.status, mo.created_at, mo.sub_total, mo.total_discount,
			mo.total_tax, mo.total_billing, mo.flag_inclusive_tax, mo.customer_phone_number,
			mo.branch_id, mb.name AS branch_name, mo.visit_purpose_id, mvp.name AS visit_purpose_name
		FROM mb_order mo
		LEFT JOIN master_branch mb ON mb.id = mo.branch_id
		LEFT JOIN master_visit_purpose mvp ON mvp.id = mo.visit_purpose_id
		WHERE mo.order_number = ?
	`, orderNumber).Scan(ctx, &header)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("order tidak ditemukan"))
		}
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data order"))
	}

	items, payment, err := resolveOrderDetailCore(ctx, h.db, orderNumber, header.Status)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data item order"))
	}

	return c.JSON(res.Success().SetData(orderDetailResult{
		OrderNumber:         header.OrderNumber,
		Status:              header.Status,
		CreatedAt:           header.CreatedAt,
		BranchID:            header.BranchID,
		BranchName:          header.BranchName,
		VisitPurposeID:      header.VisitPurposeID,
		VisitPurposeName:    header.VisitPurposeName,
		CustomerPhoneNumber: header.CustomerPhoneNumber,
		FlagInclusiveTax:    header.FlagInclusiveTax,
		SubTotal:            header.SubTotal,
		TotalDiscount:       header.TotalDiscount,
		TotalTax:            header.TotalTax,
		TotalBilling:        header.TotalBilling,
		Items:               items,
		Payment:             payment,
	}))
}

// resolveOrderDetailCore: items+packages+payment INTI GetDetail() di atas, DIPISAH (2026-09-17)
// biar dipakai BARENG versi QR Order (order_qr_detail_handler.go, GetDetail()) TANPA duplikasi
// query mb_order_detail/_package & sync ke service payment -- keduanya PERSIS sama di sini, yang
// beda cuma header & cara ngecek kepemilikan (member_id+token vs order_source+branch), makanya
// tetep di query terpisah masing-masing pemanggil (lihat qrOrderDetailHeader). status yang
// dioper = mb_order.status TERBARU, dipakai basis SyncPaymentStatus() persis kayak sebelum
// diekstrak. Error di sini SELALU error DB beneran (query item/package gagal) -- gak ada cabang
// "business validation" kayak resolveVisitPurposeDetail()/resolvePaymentMethodList(), makanya
// gak butuh return errMsg terpisah.
func resolveOrderDetailCore(ctx context.Context, db *bun.DB, orderNumber, status string) ([]orderDetailItem, orderDetailPayment, error) {
	// mod.menu_id/modp.menu_id = item_conversion_detail_id (2026-09-21, dibenerin) -- resolve
	// mi.item_name HARUS lewat master_item_conversion_detail dulu, BUKAN JOIN langsung
	// mi.id = mod.menu_id (order lama sebelum fix ini disengaja dibiarkan salah, gak dimigrasi).
	items := []orderDetailItem{}
	if err := db.NewRaw(`
		SELECT mod.ulid, mod.menu_id, mi.item_name, mod.qty, mod.notes, mod.price, mod.tax_type, mod.tax_rate,
			mod.dpp, mod.net_dpp, mod.tax_amount, mod.total, mod.promo_id, mod.discount_percent, mod.discount_amount
		FROM mb_order_detail mod
		LEFT JOIN master_item_conversion_detail micd ON micd.id = mod.menu_id
		LEFT JOIN master_item mi ON mi.id = micd.item_id
		WHERE mod.order_number = ?
		ORDER BY mod.created_at ASC
	`, orderNumber).Scan(ctx, &items); err != nil {
		return nil, orderDetailPayment{}, err
	}

	if len(items) > 0 {
		ulids := make([]string, 0, len(items))
		for _, item := range items {
			ulids = append(ulids, item.ULID)
		}

		packages := []orderDetailPackageRow{}
		if err := db.NewRaw(`
			SELECT modp.mb_order_detail_ulid, modp.menu_package_id, modp.menu_id, mi.item_name,
				modp.qty, modp.price, modp.tax_type, modp.tax_rate, modp.dpp, modp.net_dpp, modp.tax_amount, modp.total
			FROM mb_order_detail_package modp
			LEFT JOIN master_item_conversion_detail micd ON micd.id = modp.menu_id
			LEFT JOIN master_item mi ON mi.id = micd.item_id
			WHERE modp.mb_order_detail_ulid IN (?)
			ORDER BY modp.mb_order_detail_ulid ASC
		`, bun.In(ulids)).Scan(ctx, &packages); err != nil {
			return nil, orderDetailPayment{}, err
		}

		packagesByParent := map[string][]orderDetailPackageItem{}
		for _, p := range packages {
			packagesByParent[p.ParentULID] = append(packagesByParent[p.ParentULID], p.orderDetailPackageItem)
		}
		for i := range items {
			pkgs := packagesByParent[items[i].ULID]
			if pkgs == nil {
				pkgs = []orderDetailPackageItem{}
			}
			items[i].Packages = pkgs
		}
	}

	payment := orderDetailPayment{Status: status}
	syncStatus, gatewayResp, errMsg, syncErr := SyncPaymentStatus(ctx, db, orderNumber, status)
	if syncErr == nil && errMsg == "" {
		payment.Status = syncStatus
		if syncStatus == "pending" && gatewayResp != nil {
			payment.VendorQRString = gatewayResp.VendorQRString
			payment.VendorQRURL = gatewayResp.VendorQRURL
			payment.ExpiredAt = gatewayResp.ExpiredAt
		}
	}
	// SyncPaymentStatus gagal (network/db) atau belum pernah ada attempt sama sekali -- BUKAN
	// dianggap fatal (beda dari CheckPaymentStatus() yang emang tujuan utamanya itu) -- tetap
	// balikin detail order-nya, payment.status fallback ke status mb_order apa adanya, tanpa QR.

	var pmRow struct {
		PaymentMethodID   *int64  `bun:"payment_method_id"`
		PaymentMethodName *string `bun:"name"`
	}
	if err := db.NewRaw(`
		SELECT latest_pr.payment_method_id, mpm.name
		FROM (
			SELECT payment_method_id FROM mb_order_payment_request
			WHERE order_number = ? ORDER BY created_at DESC LIMIT 1
		) latest_pr
		LEFT JOIN master_payment_method mpm ON mpm.id = latest_pr.payment_method_id
	`, orderNumber).Scan(ctx, &pmRow); err == nil {
		payment.PaymentMethodID = pmRow.PaymentMethodID
		payment.PaymentMethodName = pmRow.PaymentMethodName
	}

	return items, payment, nil
}
