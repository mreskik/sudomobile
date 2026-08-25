package order

import (
	"context"
	"math"
	"strconv"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"
	"sudomobile/backend/pricing"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

// createOrderRequest: EMBED calculateRequest apa adanya (branch_id/visit_purpose_id/items/
// use_promo_ids) + field tambahan yang cuma relevan pas beneran submit (bukan preview). Body
// Calculate() jadi SUBSET valid dari body Create() -- konsisten sama prinsip "1 payload buat 2
// endpoint" yang udah dipegang dari awal.
type createOrderRequest struct {
	calculateRequest
	PaymentMethodID     int64  `json:"payment_method_id"`
	CustomerPhoneNumber string `json:"customer_phone_number"`
}

type paymentInfo struct {
	Status         string  `json:"status"`
	VendorQRString *string `json:"vendor_qr_string"`
	VendorQRURL    *string `json:"vendor_qr_url"`
	ExpiredAt      *string `json:"expired_at"`
	FailureReason  *string `json:"failure_reason"`
}

type createOrderResult struct {
	OrderNumber   string           `json:"order_number"`
	Status        string           `json:"status"`
	SubTotal      string           `json:"sub_total"`
	TotalTax      string           `json:"total_tax"`
	TotalDiscount string           `json:"total_discount"`
	TotalBilling  string           `json:"total_billing"`
	Items         []calculatedItem `json:"items"`
	Payment       paymentInfo      `json:"payment"`
}

// Create: SAVE ORDER beneran -- 1 call dari sisi client, tapi internal-nya 2 langkah backend
// (konfirmasi 2026-08-24): (1) insert mb_order+mb_order_detail(+_package) dalam 1 transaksi,
// pakai ULANG calculateOrder() yang SAMA PERSIS dipakai Calculate() (harga/pajak/promo di
// preview keranjang GAK PERNAH beda sama yang beneran ke-charge, karena literally fungsi yang
// sama); (2) minta QR ke service `payment` (dev/payment/), insert mb_order_payment_request
// (attempt tracking, mirror tr_kiosk_payment_request POS -- lihat migration
// cmd/migration/119_create_table_mb_order.sql buat catatan desainnya).
//
// order_type di-hardcode "takeaway", pax dibiarin NULL -- keputusan 2026-08-24 (mobile gak ada
// dine-in). order_fee/service_charge/platform_fee/delivery_cost mb_order disimpen 0 -- SAMA
// PERSIS gap yang udah didokumentasikan di Tahap 2 visit-purpose-detail (service_charge
// diresolve tapi gak pernah diterapkan ke perhitungan manapun di seluruh ekosistem ini, bukan
// hal baru yang kelewat di sini).
//
// Kalau step (2) gagal (service payment down/error), order TETAP kebuat (transaksi step 1 udah
// commit, order itu valid) -- cuma payment.status di response bakal "failed" dan
// vendor_qr_string/url kosong. Retry payment request buat order yang udah ada BELUM ada
// endpoint terpisahnya (di luar scope saat ini) -- dicatat sebagai next step, bukan silently
// unhandled.
func (h *handler) Create(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var body createOrderRequest
	if err := c.Bind().Body(&body); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body tidak valid"))
	}
	if body.BranchID == 0 || body.VisitPurposeID == 0 {
		return c.JSON(res.SetCode(100).SetMessage("branch_id dan visit_purpose_id wajib diisi"))
	}
	if len(body.Items) == 0 {
		return c.JSON(res.SetCode(100).SetMessage("items tidak boleh kosong"))
	}
	if body.PaymentMethodID == 0 {
		return c.JSON(res.SetCode(100).SetMessage("payment_method_id wajib diisi"))
	}

	ctx := c.Context()

	calcResult, errMsg, err := calculateOrder(ctx, h.db, body.calculateRequest, memberID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal menghitung order"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	paymentMethod, err := pricing.ResolvePaymentMethod(ctx, h.db, body.PaymentMethodID, body.BranchID, body.VisitPurposeID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data payment method"))
	}
	if paymentMethod == nil {
		return c.JSON(res.SetCode(100).SetMessage("payment method tidak ditemukan / tidak berlaku"))
	}

	var companyID *int
	if err := h.db.NewRaw(`SELECT company_id FROM master_branch WHERE id = ?`, body.BranchID).Scan(ctx, &companyID); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("branch tidak ditemukan"))
	}

	orderNumber := generateOrderNumber(body.BranchID)

	if err := insertOrder(ctx, h.db, orderNumber, memberID, companyID, body, calcResult); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal menyimpan order"))
	}

	payment := requestPaymentForOrder(ctx, h.db, orderNumber, body.BranchID, body.PaymentMethodID, paymentMethod.PaymentGatewayCode, calcResult.TotalBilling)

	return c.JSON(res.Success().SetData(createOrderResult{
		OrderNumber:   orderNumber,
		Status:        "pending",
		SubTotal:      calcResult.SubTotal,
		TotalTax:      calcResult.TotalTax,
		TotalDiscount: calcResult.TotalDiscount,
		TotalBilling:  calcResult.TotalBilling,
		Items:         calcResult.Items,
		Payment:       payment,
	}))
}

// insertOrder: 1 transaksi -- mb_order + mb_order_detail + mb_order_detail_package. Kalau ada
// yang gagal di tengah, semua di-rollback (order gak boleh nyangkut separuh jadi).
func insertOrder(ctx context.Context, db *bun.DB, orderNumber string, memberID int64, companyID *int, body createOrderRequest, result *calculateResult) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewRaw(`
			INSERT INTO mb_order (
				order_number, branch_id, member_id, visit_purpose_id, order_type, pax, status,
				order_fee, service_charge, platform_fee, delivery_cost,
				sub_total, total_discount, total_tax, total_billing,
				flag_inclusive_tax, customer_phone_number, company_id
			) VALUES (?, ?, ?, ?, 'takeaway', NULL, 'pending', 0, 0, 0, 0, ?, ?, ?, ?, ?, ?, ?)
		`, orderNumber, body.BranchID, memberID, body.VisitPurposeID,
			result.SubTotal, result.TotalDiscount, result.TotalTax, result.TotalBilling,
			result.FlagInclusiveTax, nullIfEmpty(body.CustomerPhoneNumber), companyID,
		).Exec(ctx)
		if err != nil {
			return err
		}

		for _, item := range result.Items {
			detailULID := generateULID()
			_, err := tx.NewRaw(`
				INSERT INTO mb_order_detail (
					ulid, order_number, pricelist_detail_id, menu_id, category_id, subcategory_id,
					qty, flag_inclusive_tax, price, tax_id, tax_type, tax_rate, tax_amount,
					dpp, net_dpp, promo_id, discount_percent, discount_amount, total, notes
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, detailULID, orderNumber, item.PricelistDetailID, item.MenuID, item.CategoryID, item.SubcategoryID,
				item.Qty, result.FlagInclusiveTax, item.Price, item.TaxID, nullIfEmpty(item.TaxType), item.TaxRate, item.TaxAmount,
				item.DPP, item.NetDPP, item.PromoID, item.DiscountPercent, item.DiscountAmount, item.Total, nullIfEmpty(item.Notes),
			).Exec(ctx)
			if err != nil {
				return err
			}

			for _, pkg := range item.Packages {
				_, err := tx.NewRaw(`
					INSERT INTO mb_order_detail_package (
						ulid, mb_order_detail_ulid, menu_package_id, menu_id, category_id, subcategory_id,
						qty, flag_inclusive_tax, price, tax_id, tax_type, tax_rate, tax_amount,
						dpp, net_dpp, total, notes
					) VALUES (?, ?, ?, ?, NULL, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
				`, generateULID(), detailULID, pkg.MenuPackageID, pkg.ItemID,
					pkg.Qty, result.FlagInclusiveTax, pkg.Price, pkg.TaxID, nullIfEmpty(pkg.TaxType), pkg.TaxRate, pkg.TaxAmount,
					pkg.DPP, pkg.NetDPP, pkg.Total,
				).Exec(ctx)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// requestPaymentForOrder: insert mb_order_payment_request SEBELUM manggil service payment
// (status 'pending' dulu), baru manggil -- mirror PERSIS pola Kiosk POS
// (App\Services\PaymentGatewayServices::RequestPayment(), lihat
// POS/posv1-laravel/DOKUMENTASI API/KIOSK/KIOSK PAYMENT REQUEST.md): kalau ternyata gagal,
// baris di-update jadi 'failed', BUKAN dibiarin nyangkut 'pending' palsu. Gagal request payment
// TIDAK membatalkan order yang udah ke-insert -- order tetap valid, cuma responsenya kasih tau
// payment-nya gagal (client bisa coba lagi lewat mekanisme retry yang belum dibangun).
func requestPaymentForOrder(ctx context.Context, db *bun.DB, orderNumber string, branchID int, paymentMethodID int64, gatewayCode string, totalBillingStr string) paymentInfo {
	amountFloat, _ := strconv.ParseFloat(totalBillingStr, 64)
	amountInt := int64(math.Round(amountFloat))

	_, err := db.NewRaw(`
		INSERT INTO mb_order_payment_request (order_id, order_number, payment_method_id, amount, status)
		VALUES (?, ?, ?, ?, 'pending')
	`, orderNumber, orderNumber, paymentMethodID, totalBillingStr).Exec(ctx)
	if err != nil {
		reason := "gagal menyimpan attempt pembayaran"
		return paymentInfo{Status: "failed", FailureReason: &reason}
	}

	gatewayResp, err := requestQrisPayment(orderNumber, gatewayCode, amountInt, branchID)
	if err != nil {
		reason := err.Error()
		_, _ = db.NewRaw(`UPDATE mb_order_payment_request SET status = 'failed', updated_at = now() WHERE order_id = ?`, orderNumber).Exec(ctx)
		return paymentInfo{Status: "failed", FailureReason: &reason}
	}

	_, _ = db.NewRaw(`
		UPDATE mb_order_payment_request SET expired_at = ?, updated_at = now() WHERE order_id = ?
	`, gatewayResp.ExpiredAt, orderNumber).Exec(ctx)

	return paymentInfo{
		Status:         "pending",
		VendorQRString: gatewayResp.VendorQRString,
		VendorQRURL:    gatewayResp.VendorQRURL,
		ExpiredAt:      gatewayResp.ExpiredAt,
	}
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
