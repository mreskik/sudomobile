package topup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/uptrace/bun"
)

// generateTopupReference: "TU" + branch_code + timestamp(YmdHis) + 2 digit random -- pola SAMA
// generateReferenceNumber() di modules/order/generators.go (order_number/payment_number),
// duplikasi sengaja (package beda, belum ada shared helper lintas-module di project ini).
// branch_code di-resolve dari branch_id yang dikirim client (lihat createTopupRequest).
func generateTopupReference(branchCode string) string {
	return "TU" + branchCode + time.Now().Format("20060102150405") + fmt.Sprintf("%02d", rand.Intn(100))
}

// resolvePaymentGatewayCode: payment_method_id -> payment_gateway_code, RESOLVE SERVER-SIDE
// (master_payment_method) -- gak pernah dipercaya dari client langsung, sama pola
// APIANDORDER.resolvePaymentGatewayCode() (Kiosk/POS, diperbaiki bareng 2026-09-22). Balikin
// (_, "pesan", nil) buat kondisi bisnis (gak ketemu/gak didukung), (_, "", err) buat error DB.
func resolvePaymentGatewayCode(ctx context.Context, db *bun.DB, paymentMethodID int64) (string, string, error) {
	var code sql.NullString
	err := db.NewRaw(`SELECT payment_gateway_code FROM master_payment_method WHERE id = ?`, paymentMethodID).Scan(ctx, &code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "payment method tidak ditemukan", nil
		}
		return "", "", err
	}
	if !code.Valid || code.String == "" {
		return "", "payment method tidak didukung", nil
	}
	return code.String, "", nil
}

// CreateTopup: bikin percobaan top-up baru -- SELALU lewat payment gateway (gak ada jalur tunai
// kayak Kiosk/POS APIANDORDER, customer app gak pernah pegang uang fisik). Insert
// member_topup_online 'pending' dulu, baru minta QR ke service `payment`. member_balance_ledger
// BELUM disentuh sampai settlement confirmed (lihat CheckStatus()) -- sama pola persis
// order.Create()/APIANDORDER createGatewayTopup().
//
// payment_gateway_code (2026-09-22) di-resolve SERVER-SIDE di sini dari req.PaymentMethodID --
// BUKAN dipercaya dari client. payment_method_id yang disimpan ke kolom
// member_topup_online.payment_method_id (kunci resolve COA di memberbalancejurnal, lihat
// resolvePaymentGatewayCode()).
func CreateTopup(ctx context.Context, db *bun.DB, memberID int64, req createTopupRequest) (*createTopupResponse, string, error) {
	if req.BranchID == 0 {
		return nil, "branch_id wajib diisi", nil
	}
	// decimal, BUKAN float64 -- ini duit, presisi gak boleh keganggu floating-point (sama pola
	// decimal.NewFromString() yang dipakai luas di sudocore2 buat urusan nominal/akuntansi).
	amount, err := decimal.NewFromString(strings.TrimSpace(req.Amount))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return nil, "amount wajib lebih dari 0", nil
	}
	if req.PaymentMethodID == 0 {
		return nil, "payment_method_id wajib diisi", nil
	}

	var companyID *int
	var branchCode string
	err = db.NewRaw(`SELECT company_id, COALESCE(code, '') FROM master_branch WHERE id = ?`, req.BranchID).Scan(ctx, &companyID, &branchCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "branch tidak ditemukan", nil
		}
		return nil, "", err
	}
	if branchCode == "" {
		return nil, "", fmt.Errorf("branch id %d belum punya kode (master_branch.code kosong)", req.BranchID)
	}

	paymentGatewayCode, errMsg, err := resolvePaymentGatewayCode(ctx, db, req.PaymentMethodID)
	if err != nil || errMsg != "" {
		return nil, errMsg, err
	}

	referenceNumber := generateTopupReference(branchCode)
	// amount di-normalisasi ulang jadi 2 desimal (bukan req.Amount apa adanya) -- client bisa
	// kirim format bebas ("100000", "100000.5", dst), disamain dulu biar konsisten sama kolom
	// NUMERIC(20,2) di DB.
	amountStr := amount.StringFixed(2)
	now := time.Now()

	topupRow := MemberTopupOnlineModel{
		MemberID:           memberID,
		BranchID:           &req.BranchID,
		ReferenceNumber:    referenceNumber,
		Amount:             amountStr,
		Source:             "mobile",
		PaymentMethodID:    &req.PaymentMethodID,
		PaymentGatewayCode: &paymentGatewayCode,
		Status:             "pending",
		Notes:              req.Notes,
		CompanyID:          companyID,
		CreatedAt:          now,
	}
	if _, err := db.NewInsert().Model(&topupRow).Exec(ctx); err != nil {
		return nil, "", err
	}

	amountInt := amount.Round(0).IntPart()
	gatewayResp, err := requestQrisPayment(referenceNumber, paymentGatewayCode, amountInt, req.BranchID)
	if err != nil {
		// gagal minta QR -- attempt ini gak jadi kepakai, jangan nyangkut 'pending' palsu (sama
		// pola createGatewayTopup() APIANDORDER).
		_, _ = db.NewUpdate().Model((*MemberTopupOnlineModel)(nil)).
			Set("status = ?", "failed").
			Where("reference_number = ?", referenceNumber).Exec(ctx)
		return nil, "", fmt.Errorf("gagal menghubungi payment gateway: %w", err)
	}

	var expiredAt *time.Time
	if gatewayResp.ExpiredAt != nil {
		if t, err := time.Parse(time.RFC3339, *gatewayResp.ExpiredAt); err == nil {
			expiredAt = &t
		}
	}
	if _, err := db.NewUpdate().Model((*MemberTopupOnlineModel)(nil)).
		Set("expired_at = ?", expiredAt).
		Where("reference_number = ?", referenceNumber).Exec(ctx); err != nil {
		return nil, "", err
	}

	return &createTopupResponse{
		ReferenceNumber: referenceNumber,
		Status:          "pending",
		QRString:        gatewayResp.VendorQRString,
		QRURL:           gatewayResp.VendorQRURL,
		ExpiredAt:       gatewayResp.ExpiredAt,
	}, "", nil
}

// CheckTopupStatus: polling status top-up gateway -- dipanggil sambil QR ditampilin ke customer.
// Begitu settlement confirmed, baru insert member_balance_ledger (saldo update). Idempotency
// guard: status lokal udah 'paid' -> gak query ulang ke gateway / gak insert ledger dobel. Order
// tanpa member (topup selalu wajib login, lihat handler) -- referenceNumber dicek milik memberID
// yang minta, sama alasan payment-status order (customer-facing, gak boleh bisa ngintip/nge-poll
// punya orang lain).
func CheckTopupStatus(ctx context.Context, db *bun.DB, memberID int64, referenceNumber string) (*checkTopupStatusResponse, string, error) {
	var topupRow MemberTopupOnlineModel
	err := db.NewSelect().Model(&topupRow).Where("reference_number = ?", referenceNumber).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "topup tidak ditemukan", nil
		}
		return nil, "", err
	}
	if topupRow.MemberID != memberID {
		return nil, "topup tidak ditemukan", nil
	}

	if topupRow.Status == "paid" {
		balanceAfter, err := getLastBalance(ctx, db, memberID)
		if err != nil {
			return nil, "", err
		}
		return &checkTopupStatusResponse{ReferenceNumber: referenceNumber, Status: "paid", BalanceAfter: &balanceAfter}, "", nil
	}
	if topupRow.Status != "pending" {
		return &checkTopupStatusResponse{ReferenceNumber: referenceNumber, Status: topupRow.Status}, "", nil
	}

	gatewayResp, err := getPaymentGatewayStatus(referenceNumber)
	if err != nil {
		return nil, "", err
	}
	status := gatewayResp.Status

	// FALLBACK: expired_at lokal udah lewat tapi gateway masih bilang 'pending' -- mirror pola
	// APIANDORDER CheckStatus() (webhook mungkin gak akan pernah nyampe).
	if status == "pending" && topupRow.ExpiredAt != nil && time.Now().After(*topupRow.ExpiredAt) {
		_ = cancelPaymentGateway(referenceNumber)
		status = "expired"
	}

	if status == "settlement" {
		balanceAfter, err := confirmTopupPaid(ctx, db, &topupRow)
		if err != nil {
			return nil, "", err
		}
		return &checkTopupStatusResponse{ReferenceNumber: referenceNumber, Status: "paid", BalanceAfter: &balanceAfter}, "", nil
	}

	if status == "expired" || status == "cancel" || status == "failed" {
		now := time.Now()
		_, _ = db.NewUpdate().Model((*MemberTopupOnlineModel)(nil)).
			Set("status = ?", status).
			Set("cancel_at = ?", now).
			Where("reference_number = ? AND status = ?", referenceNumber, "pending").Exec(ctx)
	}

	return &checkTopupStatusResponse{ReferenceNumber: referenceNumber, Status: status}, "", nil
}

// confirmTopupPaid: tandain member_topup_online 'paid' + insert member_balance_ledger, 1
// transaksi. Guard status = 'pending' di WHERE update jaga-jaga race (2 request check-status
// bersamaan) -- kalau RowsAffected() = 0, berarti udah kepake proses lain duluan, ambil saldo
// terkini aja tanpa insert ledger baru. Sama persis pola confirmTopupPaid() APIANDORDER.
func confirmTopupPaid(ctx context.Context, db *bun.DB, topupRow *MemberTopupOnlineModel) (string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	gagal := true
	defer func() {
		if gagal {
			tx.Rollback()
		}
	}()

	now := time.Now()
	res, err := tx.NewUpdate().Model((*MemberTopupOnlineModel)(nil)).
		Set("status = ?", "paid").
		Set("paid_at = ?", now).
		Where("reference_number = ? AND status = ?", topupRow.ReferenceNumber, "pending").
		Exec(ctx)
	if err != nil {
		return "", err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		gagal = false
		tx.Rollback()
		return getLastBalance(ctx, db, topupRow.MemberID)
	}

	balanceAfter, err := lockMemberAndInsertLedger(ctx, tx, topupRow.MemberID, topupRow.BranchID, topupRow.ReferenceNumber, topupRow.Amount)
	if err != nil {
		return "", err
	}

	gagal = false
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return balanceAfter, nil
}

// lockMemberAndInsertLedger: lock baris master_member (serialize operasi saldo per-member, nyegah
// race antar 2 transaksi barengan buat member yang sama), insert baris baru dengan
// balance_after = lama + balance_in (dihitung di SQL, bukan Go float, biar presisi NUMERIC-nya
// gak keganggu). Sama persis pola lockMemberAndInsertLedger() APIANDORDER, source = 'mobile'.
func lockMemberAndInsertLedger(ctx context.Context, tx bun.Tx, memberID int64, branchID *int64, referenceNumber, balanceIn string) (string, error) {
	var dummy int64
	if err := tx.NewRaw(`SELECT id FROM master_member WHERE id = ? FOR UPDATE`, memberID).Scan(ctx, &dummy); err != nil {
		return "", fmt.Errorf("member tidak ditemukan: %w", err)
	}

	var balanceAfter string
	err := tx.NewRaw(`
		INSERT INTO member_balance_ledger
			(member_id, branch_id, transaction_type, source, reference_number, balance_in, balance_out, balance_after)
		VALUES (?, ?, 'topup', 'mobile', ?, ?, 0,
			COALESCE((SELECT balance_after FROM member_balance_ledger WHERE member_id = ? AND is_deleted = false ORDER BY created_at DESC, id DESC LIMIT 1), 0) + ?)
		RETURNING balance_after
	`, memberID, branchID, referenceNumber, balanceIn, memberID, balanceIn).Scan(ctx, &balanceAfter)
	if err != nil {
		return "", err
	}
	return balanceAfter, nil
}

func getLastBalance(ctx context.Context, db *bun.DB, memberID int64) (string, error) {
	var balance string
	err := db.NewRaw(`
		SELECT balance_after FROM member_balance_ledger
		WHERE member_id = ? AND is_deleted = false
		ORDER BY created_at DESC, id DESC LIMIT 1
	`, memberID).Scan(ctx, &balance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "0.00", nil
		}
		return "", err
	}
	return balance, nil
}

// GetTopupHistory: SEMUA percobaan top-up member yang lagi login, terbaru duluan -- BEDA dari
// BALANCE HISTORY.md (baca member_balance_ledger, cuma transaksi yang UDAH settlement). Di sini
// baca member_topup_online LANGSUNG, semua status (pending/paid/expired/cancel/failed) ikut
// muncul -- customer bisa liat "topup gue kemarin kenapa gak masuk-masuk". start_date/end_date
// OPSIONAL, kosong dua-duanya = default HARI INI (SAMA aturan BalanceHistory()/PointHistory()
// account/balance_handler.go -- biar gak narik seluruh histori tanpa sengaja).
func GetTopupHistory(ctx context.Context, db *bun.DB, memberID int64, startDate, endDate string) ([]topupHistoryRow, error) {
	list := []topupHistoryRow{}
	err := db.NewRaw(`
		SELECT reference_number, amount, status, created_at, paid_at
		FROM member_topup_online
		WHERE member_id = ?
		  AND created_at >= ?::date
		  AND created_at < (?::date + interval '1 day')
		ORDER BY created_at DESC, id DESC
	`, memberID, startDate, endDate).Scan(ctx, &list)
	return list, err
}
