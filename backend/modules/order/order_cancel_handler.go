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

type cancelOrderRequest struct {
	Notes string `json:"notes"`
}

// CancelOrder: POST /api/order/:order_number/cancel -- batalin order SEBELUM bayar. Sistem
// order di sini "sekali jalan" (item+diskon+payment_method udah final pas create-order, gak
// ada hold/edit kayak POS kasir) -- jadi cancel-nya juga simpel: langsung ubah status order,
// gak ada state "hold" yang perlu ditangani terpisah kayak OrderServices::CancelOrder() POS.
//
// TETEP mirror bagian PALING PENTING dari pola POS: race guard sebelum cancel attempt payment
// yang masih pending (lihat cancelPendingAttempt()) -- kalau ternyata customer keburu bayar
// PERSIS pas mau di-cancel, order otomatis di-finalize jadi 'paid' (BUKAN di-cancel), biar duit
// yang udah beneran kebayar gak pernah "ke-orphan" (Midtrans nerima, tapi mb_order gak pernah
// ke-mark paid).
func (h *handler) CancelOrder(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)
	orderNumber := c.Params("order_number")

	var body cancelOrderRequest
	_ = c.Bind().Body(&body) // notes opsional, body kosong/gak ada tetep valid

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
	if order.Status != "pending" {
		return c.JSON(res.SetCode(100).SetMessage("bukan order pending, gak bisa di-cancel"))
	}

	alreadyPaid, err := cancelPendingAttempt(ctx, h.db, orderNumber)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal cancel attempt pembayaran"))
	}
	if alreadyPaid {
		return c.JSON(res.SetCode(100).SetMessage("order ternyata sudah dibayar, tidak jadi di-cancel"))
	}

	_, err = h.db.NewRaw(`
		UPDATE mb_order SET status = 'cancel', cancel_at = now(), cancel_notes = ?, updated_at = now()
		WHERE order_number = ? AND status = 'pending'
	`, nullIfEmpty(body.Notes), orderNumber).Exec(ctx)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal cancel order"))
	}

	return c.JSON(res.Success().SetMessage("cancel order berhasil"))
}

// cancelPendingAttempt: cancel attempt payment TERBARU kalau statusnya masih 'pending' --
// no-op (gak ada yang perlu di-cancel) kalau belum pernah ada attempt sama sekali, atau attempt
// terakhir udah gak 'pending' lagi (udah settlement/expired/cancel/failed duluan).
//
// RACE GUARD (mirror PaymentGatewayServices::CancelPendingAttempt() POS): live-check ke gateway
// DULU sebelum mutusin cancel -- bukan blind-cancel. Kalau ternyata udah 'settlement' (customer
// beneran bayar PERSIS pas endpoint ini diproses), payment di-finalize (insert mb_order_payment
// + mb_order.status='paid') lewat fungsi yang SAMA dipakai CheckPaymentStatus(), balikin
// alreadyPaid=true biar pemanggil TIDAK lanjut cancel order-nya. Kalau live-check-nya sendiri
// gagal (network dll), diperlakukan kayak "masih pending" -- tetap dicoba di-cancel ke gateway
// (Midtrans yang nolak kalau ternyata udah selesai duluan).
func cancelPendingAttempt(ctx context.Context, db *bun.DB, orderNumber string) (alreadyPaid bool, err error) {
	var attempt struct {
		OrderID         string `bun:"order_id"`
		Status          string `bun:"status"`
		PaymentMethodID int64  `bun:"payment_method_id"`
		Amount          string `bun:"amount"`
	}
	err = db.NewRaw(`
		SELECT order_id, status, payment_method_id, amount FROM mb_order_payment_request
		WHERE order_number = ? ORDER BY created_at DESC LIMIT 1
	`, orderNumber).Scan(ctx, &attempt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if attempt.Status != "pending" {
		return false, nil
	}

	gatewayResp, statusErr := getPaymentGatewayStatus(attempt.OrderID)
	if statusErr == nil && gatewayResp.Status == "settlement" {
		if err := finalizeSettledPayment(ctx, db, orderNumber, attempt.OrderID, attempt.PaymentMethodID, attempt.Amount); err != nil {
			return false, err
		}
		return true, nil
	}
	if statusErr == nil && gatewayResp.Status != "pending" {
		// udah expired/cancel/failed duluan di gateway (bukan gara-gara request ini) -- gak
		// perlu cancel lagi, sinkronin status lokal aja apa adanya.
		_, _ = db.NewRaw(`UPDATE mb_order_payment_request SET status = ?, updated_at = now() WHERE order_id = ?`, gatewayResp.Status, attempt.OrderID).Exec(ctx)
		return false, nil
	}

	if err := cancelPaymentGateway(attempt.OrderID); err != nil {
		return false, err
	}
	_, _ = db.NewRaw(`UPDATE mb_order_payment_request SET status = 'cancel', updated_at = now() WHERE order_id = ?`, attempt.OrderID).Exec(ctx)
	return false, nil
}
