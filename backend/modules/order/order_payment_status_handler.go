package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"sudomobile/backend/helpers"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type paymentStatusResult struct {
	OrderNumber string `json:"order_number"`
	Status      string `json:"status"`
}

// orderOwnerRow: hasil lookup mb_order (member_id + status). MemberID *int64 (2026-09-17,
// migration sudocore2 208) -- mb_order.member_id sekarang NULLABLE (order tamu, QR Order maupun
// member app yang gak login), pointer WAJIB biar gak Scan error.
//
// PUBLIK (2026-09-22) -- endpoint ini gak lagi cek kepemilikan (dulu lewat isMemberOwner(), sudah
// dihapus). order_number sendiri jadi kunci akses: siapa pun yang tau/pegang nomornya berhak cek
// status pembayarannya -- lihat catatan di router.go.
type orderOwnerRow struct {
	MemberID *int64 `bun:"member_id"`
	Status   string `bun:"status"`
}

// CheckPaymentStatus: GET /api/order/:order_number/payment-status -- dipanggil buat POLLING
// (mis. tiap beberapa detik) sambil QR ditampilin ke customer. Mirror PERSIS alur
// PaymentGatewayServices::CheckStatus() POS (lihat KIOSK PAYMENT CHECK STATUS.md).
//
// Alur:
//  1. Idempotency guard -- kalau mb_order.status udah 'paid', langsung balikin 'paid' TANPA
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
//     sini, sinkronisasi yang sama juga dijalanin background job `orderstatuschanger` (5 menit
//     sekali, lihat DOKUMENTASI BACKGROUND JOB/ORDER STATUS CHANGER.md) -- jaring pengaman buat
//     order yang customer-nya ninggalin app dan gak pernah polling lagi.
func (h *handler) CheckPaymentStatus(c fiber.Ctx) error {
	res := helpers.NewResponse()
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
		if err := expireOrderAndRefundPoint(ctx, db, orderNumber); err != nil {
			return "", nil, "", err
		}
		return "expired", gatewayResp, "", nil
	default:
		// pending / cancel / failed -- dibalikin apa adanya, gak ada state mb_order yang perlu
		// disinkronin (pending tetep pending, cancel/failed nunggu attempt baru kalau ada retry).
		return gatewayResp.Status, gatewayResp, "", nil
	}
}

// finalizeSettledPayment: generate payment_number (BARU muncul di sini, pas payment beneran
// settlement -- BUKAN pas order dibuat), insert mb_order_payment (FINAL, sekali doang) + update
// mb_order.status jadi 'paid' + mb_order.payment_number. payment_amount/payment_method_id
// diambil dari mb_order_payment_request (snapshot pas request dibuat), BUKAN dari gateway/client
// -- mirror PaymentServices::SavePayment() POS.
//
// mb_order_payment.payment_number FK ke mb_order(payment_number) (bukan order_number lagi, lihat
// migration 120) -- mb_order WAJIB di-UPDATE duluan sebelum INSERT ke mb_order_payment, kebalik
// bakal kena FK violation (nilai payment_number harus udah ada di mb_order dulu).
//
// Dibungkus 1 transaksi biar update+insert konsisten (gak ada kondisi payment_number kesimpen di
// mb_order tapi mb_order_payment ketinggalan, atau sebaliknya) -- INCLUDING pg_notify() di bawah:
// NOTIFY yang dipanggil di dalam transaksi Postgres ditunda sampai COMMIT beneran kejadian, dan
// kalau transaksinya ROLLBACK, NOTIFY-nya gak pernah kekirim -- otomatis gak ada sinyal palsu
// buat order yang ternyata gagal di-finalize.
//
// pg_notify('mb_order_paid', '{branch_id}:{order_number}') -- disepakati 2026-08-26, dikonsumsi
// APIANDORDER (LISTEN, relay ke worker POS lewat WebSocket per branch). Level APLIKASI (bukan DB
// trigger) -- keputusan final, lihat CATATAN INTERNAL.md.
func finalizeSettledPayment(ctx context.Context, db *bun.DB, orderNumber, paymentGatewayOrderID string, paymentMethodID int64, amount string) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var branchID int
		var branchCode string
		if err := tx.NewRaw(`
			SELECT mo.branch_id, COALESCE(mbr.code, '') FROM mb_order mo
			JOIN master_branch mbr ON mbr.id = mo.branch_id
			WHERE mo.order_number = ?
		`, orderNumber).Scan(ctx, &branchID, &branchCode); err != nil {
			return err
		}
		paymentNumber := generatePaymentNumber(branchCode)

		if _, err := tx.NewRaw(`
			UPDATE mb_order SET status = 'paid', payment_number = ?, updated_at = now() WHERE order_number = ?
		`, paymentNumber, orderNumber).Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewRaw(`
			INSERT INTO mb_order_payment (ulid, payment_number, payment_method_id, payment_amount, payment_gateway_order_id)
			VALUES (?, ?, ?, ?, ?)
		`, generateULID(), paymentNumber, paymentMethodID, amount, paymentGatewayOrderID).Exec(ctx); err != nil {
			return err
		}

		payload := fmt.Sprintf("%d:%s", branchID, orderNumber)
		_, err := tx.NewRaw(`SELECT pg_notify('mb_order_paid', ?)`, payload).Exec(ctx)
		return err
	})
}

// expireOrderAndRefundPoint: update mb_order.status='expired' + refund poin (kalau order ini
// punya point_redeem_amount > 0, dipotong pas Create() -- lihat redeemMemberPoint()) dalam 1
// transaksi. Guard WHERE status='pending' di UPDATE mb_order -- kalau ternyata order ini udah
// ke-update status lain duluan (race, mis. keburu paid), UPDATE ini no-op (RowsAffected 0) dan
// refund SENGAJA DI-SKIP (guard eksplisit di bawah) -- order yang beneran paid gak boleh
// ke-refund poinnya cuma gara-gara job expired sweep sempet nyenggol bareng.
func expireOrderAndRefundPoint(ctx context.Context, db *bun.DB, orderNumber string) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.NewRaw(`
			UPDATE mb_order SET status = 'expired', updated_at = now() WHERE order_number = ? AND status = 'pending'
		`, orderNumber).Exec(ctx)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return nil
		}
		return refundMemberPoint(ctx, tx, orderNumber)
	})
}

// refundMemberPoint: kembalikan poin yang kepotong pas Create() -- dipanggil dari
// expireOrderAndRefundPoint() maupun CancelOrder(). Insert baris BARU (point_in, transaction_type
// 'redeem_reversal') -- TIDAK edit/hapus baris redeem lama (soft-delete-as-reversal, sama pola
// kayak Member Point Adjustment di sudocore2). No-op (bukan error) kalau order ini emang gak
// pernah py baris redeem (point_redeem_amount = 0, promo biasa tanpa syarat poin) -- guard
// idempotency-nya constraint UNIQUE(reference_number, transaction_type) WHERE mmpc_id IS NULL
// (migration 232), ON CONFLICT DO NOTHING -- refund gak bakal dobel kalau expire sweep dan
// polling manual kebetulan nyenggol bareng buat order yang sama.
func refundMemberPoint(ctx context.Context, tx bun.Tx, orderNumber string) error {
	var redeemed struct {
		MemberID  *int64  `bun:"member_id"`
		BranchID  int     `bun:"branch_id"`
		CompanyID *int    `bun:"company_id"`
		Amount    float64 `bun:"amount"`
	}
	err := tx.NewRaw(`
		SELECT mo.member_id, mo.branch_id, mo.company_id, mo.point_redeem_amount AS amount
		FROM mb_order mo WHERE mo.order_number = ? AND mo.point_redeem_amount > 0
	`, orderNumber).Scan(ctx, &redeemed)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if redeemed.MemberID == nil {
		return nil
	}

	var currentBalance float64
	err = tx.NewRaw(`
		SELECT balance_after FROM member_point_ledger
		WHERE member_id = ? AND is_deleted = false
		ORDER BY created_at DESC, id DESC LIMIT 1
	`, *redeemed.MemberID).Scan(ctx, &currentBalance)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	newBalance := currentBalance + redeemed.Amount
	_, err = tx.NewRaw(`
		INSERT INTO member_point_ledger
			(member_id, branch_id, transaction_type, reference_number, point_in, point_out, balance_after, company_id)
		VALUES (?, ?, 'redeem_reversal', ?, ?, 0, ?, ?)
		ON CONFLICT (reference_number, transaction_type) WHERE mmpc_id IS NULL DO NOTHING
	`, *redeemed.MemberID, redeemed.BranchID, orderNumber, redeemed.Amount, newBalance, redeemed.CompanyID).Exec(ctx)
	return err
}
