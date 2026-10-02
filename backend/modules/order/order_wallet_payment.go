package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/uptrace/bun"
)

// errInsufficientBalance: dikembalikan insertWalletPaidOrder() kalau re-check saldo DI DALAM
// transaksi ternyata gagal (race 2 request bareng) -- Create() nangkep ini spesifik buat
// mastiin client dapet pesan "insufficient balance" yang sama persis kayak early-reject,
// bukan pesan generic "failed to save order".
var errInsufficientBalance = errors.New("insufficient balance")

// getMemberBalance: saldo TERKINI member -- balance_after baris TERAKHIR member_balance_ledger,
// SAMA PERSIS formula account.Balance() handler (sudomobile). Dipakai 2 tempat: Create() buat
// early-reject SEBELUM buka transaksi (UX cepat, luar tx via *bun.DB) DAN insertWalletPaidOrder()
// buat re-check final DI DALAM tx (lewat bun.Tx) -- satu tempat nulis query saldo, bun.IDB
// jadi interface umum yang nerima *bun.DB maupun bun.Tx.
//
// decimal.Decimal (BUKAN float64) -- ini duit beneran, float64 rawan rounding error pas
// dibandingin (balance < totalBilling) yang hasilnya nentuin order boleh jalan apa ditolak.
func getMemberBalance(ctx context.Context, db bun.IDB, memberID int64) (decimal.Decimal, error) {
	var balanceStr string
	err := db.NewRaw(`
		SELECT balance_after FROM member_balance_ledger
		WHERE member_id = ? AND is_deleted = false
		ORDER BY created_at DESC, id DESC LIMIT 1
	`, memberID).Scan(ctx, &balanceStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}
	balance, err := decimal.NewFromString(balanceStr)
	if err != nil {
		return decimal.Zero, err
	}
	return balance, nil
}

// calculateMdrAmount: mdr_amount SNAPSHOT (migration sudocore2 242, 2026-10-01) -- payment_amount
// x master_payment_method.mdr / 100, dihitung SAAT payment dibuat. CUMA informasi, TIDAK ada
// jurnal/posting akuntansi yang nyentuh nilai ini. payment_method_id gak ketemu/mdr NULL ->
// dianggap 0 (bukan error, biar gak gagalin seluruh order gara-gara lookup mdr doang).
func calculateMdrAmount(ctx context.Context, db bun.IDB, paymentMethodID int64, paymentAmount decimal.Decimal) (decimal.Decimal, error) {
	var mdrStr sql.NullString
	err := db.NewRaw(`SELECT mdr FROM master_payment_method WHERE id = ?`, paymentMethodID).Scan(ctx, &mdrStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}
	if !mdrStr.Valid || mdrStr.String == "" {
		return decimal.Zero, nil
	}
	mdr, err := decimal.NewFromString(mdrStr.String)
	if err != nil {
		return decimal.Zero, err
	}
	return paymentAmount.Mul(mdr).Div(decimal.NewFromInt(100)).Round(2), nil
}

// insertWalletPaidOrder: versi insertOrder() KHUSUS WALLET_PAYMENT -- order settle INSTAN,
// BEDA dari insertOrder() biasa di 4 hal:
//  1. mb_order langsung status='paid' + payment_number TERISI (bukan 'pending' + NULL, gak
//     ada tahap nunggu gateway sama sekali).
//  2. Saldo DI-RE-CHECK di dalam tx (bukan cuma early-reject di Create()) -- balance TERBARU
//     dibaca ulang DI DALAM transaksi ini, race 2 request bareng (device lain/klik 2x) yang
//     lolos early-reject yang SAMA tetap ketangkep di sini & bikin SELURUH transaksi
//     di-ROLLBACK (order BATAL total, beda dari redeemMemberPoint() poin yang sengaja gak
//     re-check/boleh minus -- saldo duit beneran, gak boleh kejadian sama sekali).
//  3. INSERT member_balance_ledger (transaction_type='payment' -- SESUAI konvensi
//     memberbalancejurnal.RunOnce() sudocore2: baris 'payment' DIANGGAP udah kehandle lewat
//     jurnal endday POS, asalkan payment_method "Bayar pakai Saldo" di-setting
//     coa_accout_id = akun 2.1.07.01 Member Wallet Payable -- SETUP data, bukan kode).
//  4. INSERT mb_order_payment LANGSUNG (bukan nunggu finalizeSettledPayment() yang baru
//     jalan pas gateway settlement) -- deduct_member_id (migration sudocore2 241) diisi
//     memberID, snapshot "siapa yang saldonya kepotong".
//
// pg_notify('mb_order_paid', ...) di DALAM tx yang sama -- SAMA PERSIS finalizeSettledPayment(),
// biar cuma kekirim kalau transaksi beneran commit (NOTIFY di Postgres ditunda sampai COMMIT).
func insertWalletPaidOrder(ctx context.Context, db *bun.DB, orderNumber, paymentNumber string, memberID int64, companyID *int, body createOrderRequest, result *calculateResult) error {
	totalBilling, err := decimal.NewFromString(result.TotalBilling)
	if err != nil {
		return err
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewRaw(`
			INSERT INTO mb_order (
				order_number, branch_id, member_id, visit_purpose_id, order_type, pax, status,
				order_fee, service_charge, platform_fee, delivery_cost,
				sub_total, total_discount, total_tax, total_billing, point_redeem_amount,
				flag_inclusive_tax, customer_phone_number, company_id, order_source, order_name,
				payment_number
			) VALUES (?, ?, ?, ?, 'takeaway', NULL, 'paid', 0, 0, 0, 0, ?, ?, ?, ?, ?, ?, ?, ?, 'mobile', ?, ?)
		`, orderNumber, body.BranchID, memberID, body.VisitPurposeID,
			result.SubTotal, result.TotalDiscount, result.TotalTax, result.TotalBilling, result.PointRedeemAmount,
			result.FlagInclusiveTax, nullIfEmpty(body.CustomerPhoneNumber), companyID, nullIfEmpty(body.CustomerName),
			paymentNumber,
		).Exec(ctx)
		if err != nil {
			return err
		}

		if err := insertOrderItems(ctx, tx, orderNumber, result); err != nil {
			return err
		}

		if result.PointRedeemAmount > 0 {
			if err := redeemMemberPoint(ctx, tx, memberID, body.BranchID, companyID, orderNumber, result.PointRedeemAmount); err != nil {
				return err
			}
		}

		currentBalance, err := getMemberBalance(ctx, tx, memberID)
		if err != nil {
			return err
		}
		if currentBalance.LessThan(totalBilling) {
			return errInsufficientBalance
		}
		newBalance := currentBalance.Sub(totalBilling)

		if _, err := tx.NewRaw(`
			INSERT INTO member_balance_ledger
				(member_id, branch_id, transaction_type, source, reference_number, balance_in, balance_out, balance_after, company_id)
			VALUES (?, ?, 'payment', 'mobile', ?, 0, ?, ?, ?)
		`, memberID, body.BranchID, orderNumber, result.TotalBilling, newBalance.StringFixed(2), companyID).Exec(ctx); err != nil {
			return err
		}

		mdrAmount, err := calculateMdrAmount(ctx, tx, body.PaymentMethodID, totalBilling)
		if err != nil {
			return err
		}

		if _, err := tx.NewRaw(`
			INSERT INTO mb_order_payment (ulid, payment_number, payment_method_id, payment_amount, deduct_member_id, mdr_amount)
			VALUES (?, ?, ?, ?, ?, ?)
		`, generateULID(), paymentNumber, body.PaymentMethodID, result.TotalBilling, memberID, mdrAmount.StringFixed(2)).Exec(ctx); err != nil {
			return err
		}

		payload := fmt.Sprintf("%d:%s", body.BranchID, orderNumber)
		_, err = tx.NewRaw(`SELECT pg_notify('mb_order_paid', ?)`, payload).Exec(ctx)
		return err
	})
}
