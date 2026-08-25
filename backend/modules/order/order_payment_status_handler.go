package order

import (
	"context"
	"database/sql"
	"errors"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type paymentStatusResult struct {
	OrderNumber string `json:"order_number"`
	Status      string `json:"status"`
}

// orderOwnerRow: hasil lookup mb_order buat cek kepemilikan -- BEDA dari Kiosk (POS internal
// staff, gak ada konsep "punya siapa") -- sudomobile customer-facing, jadi WAJIB mastiin
// member yang lagi login itu emang pemilik order ini sebelum ngasih tau status pembayarannya.
type orderOwnerRow struct {
	MemberID int64  `bun:"member_id"`
	Status   string `bun:"status"`
}

// CheckPaymentStatus: GET /api/order/:order_number/payment-status -- dipanggil buat POLLING
// (mis. tiap beberapa detik) sambil QR ditampilin ke customer. Mirror PERSIS alur
// PaymentGatewayServices::CheckStatus() POS (lihat KIOSK PAYMENT CHECK STATUS.md), DITAMBAH
// pengecekan kepemilikan order (member_id harus cocok token yang login).
//
// Alur:
//  1. Cek order-nya punya member yang login (bukan cuma "ada").
//  2. Idempotency guard -- kalau mb_order.status udah 'paid', langsung balikin 'paid' TANPA
//     ngecek ulang ke gateway atau insert mb_order_payment lagi (polling berkali-kali gak
//     dobel proses).
//  3. Kalau belum, ambil attempt TERBARU dari mb_order_payment_request, live-check ke service
//     payment (GET /payment-gateway/{order_id}).
//  4. Status attempt di-update lokal sesuai hasil live-check APA ADANYA (settlement, bukan
//     'paid' -- remap cuma di response, sama kayak POS).
//  5. Kalau settlement -> insert mb_order_payment (FINAL, idempotent lewat guard status di
//     langkah 2 -- polling ulang abis ini gak bakal nyampe sini lagi) + update mb_order.status
//     jadi 'paid'.
//  6. Kalau expired -> mb_order.status ikut disinkronin jadi 'expired' (guard WHERE
//     status='pending', biar gak nabrak state lain). Selain dipicu polling manual kayak di
//     sini, sinkronisasi yang sama juga dijalanin background job `orderexpiry` (5 menit
//     sekali, lihat DOKUMENTASI BACKGROUND JOB/ORDER EXPIRY.md) -- jaring pengaman buat order
//     yang customer-nya ninggalin app dan gak pernah polling lagi.
func (h *handler) CheckPaymentStatus(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)
	orderNumber := c.Params("order_number")

	ctx := c.Context()

	var order orderOwnerRow
	err := h.db.NewRaw(`SELECT member_id, status FROM mb_order WHERE order_number = ?`, orderNumber).Scan(ctx, &order)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("order tidak ditemukan"))
		}
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data order"))
	}
	if order.MemberID != memberID {
		return c.JSON(res.SetCode(100).SetMessage("order tidak ditemukan"))
	}

	status, _, errMsg, err := SyncPaymentStatus(ctx, h.db, orderNumber, order.Status)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal cek status pembayaran"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	return c.JSON(res.Success().SetData(paymentStatusResult{OrderNumber: orderNumber, Status: status}))
}

// SyncPaymentStatus: INTI logic sinkronisasi status pembayaran, DIPISAH dari handler
// CheckPaymentStatus() biar bisa dipakai ULANG sama GetDetail() (order detail nunjukin status
// pembayaran yang SELALU fresh + QR kalau masih pending, bukan data statis lama) -- prinsip
// yang sama kayak calculateOrder() dipakai bareng Calculate()/Create().
//
// Balikin (status, gatewayResp, "", nil) kalau sukses -- gatewayResp nil kalau order udah
// 'paid' dari awal (gak sempat/gak perlu live-check ke gateway lagi, idempotency guard).
// (status, nil, "pesan", nil) kalau ada kondisi bisnis yang bikin gak bisa lanjut (belum
// pernah ada attempt payment). (_, _, "", err) kalau beneran error DB/network.
func SyncPaymentStatus(ctx context.Context, db *bun.DB, orderNumber, currentOrderStatus string) (string, *paymentGatewayResponse, string, error) {
	if currentOrderStatus == "paid" {
		return "paid", nil, "", nil
	}

	var attempt struct {
		OrderID         string `bun:"order_id"`
		PaymentMethodID int64  `bun:"payment_method_id"`
		Amount          string `bun:"amount"`
	}
	err := db.NewRaw(`
		SELECT order_id, payment_method_id, amount FROM mb_order_payment_request
		WHERE order_number = ? ORDER BY created_at DESC LIMIT 1
	`, orderNumber).Scan(ctx, &attempt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, "belum pernah ada request pembayaran buat order ini", nil
		}
		return "", nil, "", err
	}

	gatewayResp, err := getPaymentGatewayStatus(attempt.OrderID)
	if err != nil {
		return "", nil, "", err
	}

	if _, err := db.NewRaw(`
		UPDATE mb_order_payment_request SET status = ?, updated_at = now() WHERE order_id = ?
	`, gatewayResp.Status, attempt.OrderID).Exec(ctx); err != nil {
		return "", nil, "", err
	}

	switch gatewayResp.Status {
	case "settlement":
		if err := finalizeSettledPayment(ctx, db, orderNumber, attempt.OrderID, attempt.PaymentMethodID, attempt.Amount); err != nil {
			return "", nil, "", err
		}
		return "paid", gatewayResp, "", nil
	case "expired":
		_, _ = db.NewRaw(`UPDATE mb_order SET status = 'expired', updated_at = now() WHERE order_number = ? AND status = 'pending'`, orderNumber).Exec(ctx)
		return "expired", gatewayResp, "", nil
	default:
		// pending / cancel / failed -- dibalikin apa adanya, gak ada state mb_order yang perlu
		// disinkronin (pending tetep pending, cancel/failed nunggu attempt baru kalau ada retry).
		return gatewayResp.Status, gatewayResp, "", nil
	}
}

// finalizeSettledPayment: insert mb_order_payment (FINAL, sekali doang) + update mb_order.status
// jadi 'paid'. payment_amount/payment_method_id diambil dari mb_order_payment_request (snapshot
// pas request dibuat), BUKAN dari gateway/client -- mirror PaymentServices::SavePayment() POS.
// Dibungkus 1 transaksi biar insert+update konsisten (gak ada kondisi payment_amount kesimpen
// tapi mb_order.status ketinggalan 'pending', atau sebaliknya).
func finalizeSettledPayment(ctx context.Context, db *bun.DB, orderNumber, paymentGatewayOrderID string, paymentMethodID int64, amount string) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewRaw(`
			INSERT INTO mb_order_payment (ulid, order_number, payment_method_id, payment_amount, payment_gateway_order_id)
			VALUES (?, ?, ?, ?, ?)
		`, generateULID(), orderNumber, paymentMethodID, amount, paymentGatewayOrderID).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewRaw(`UPDATE mb_order SET status = 'paid', updated_at = now() WHERE order_number = ?`, orderNumber).Exec(ctx)
		return err
	})
}
