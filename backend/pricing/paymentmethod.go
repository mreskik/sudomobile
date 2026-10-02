package pricing

import (
	"context"

	"sudomobile/backend/modules/paymentmethod"

	"github.com/uptrace/bun"
)

// PaymentMethod: hasil ResolvePaymentMethod() -- 1 baris master_payment_method yang lolos
// filter yang SAMA PERSIS kayak GET .../payment-method (gateway-only ATAU WALLET_PAYMENT +
// scoped branch+visit purpose, hormatin flag_all_branch/flag_all_visitpurpose) -- lihat
// DOKUMENTASI API/MOBILE/MENU/GET PAYMENT METHOD LIST.md.
type PaymentMethod struct {
	ID                 int64  `bun:"id"`
	Name               string `bun:"name"`
	Code               string `bun:"code"`
	PaymentGatewayCode string `bun:"payment_gateway_code"`
	// PaymentMethodTypeID (2026-10-01) -- dipakai order.Create() buat deteksi WALLET_PAYMENT
	// (paymentmethod.WalletPaymentMethodTypeID) & jalanin barrier saldo khusus.
	PaymentMethodTypeID *int64 `bun:"payment_method_type_id"`
}

// ResolvePaymentMethod: nil (bukan error) kalau payment_method_id gak ketemu ATAU gak lolos
// scope (gateway-only/wallet, branch, visit_purpose) -- dipakai buat validasi pas save order
// (client gak boleh kirim payment_method_id sembarangan yang gak lolos filter yang sama kayak
// listing).
//
// Filter gateway-only DILONGGARKAN (2026-10-01, mirror paymentmethod.resolvePaymentMethodList()):
// WALLET_PAYMENT (type_id=5) SELALU lolos apa pun isi payment_gateway_code-nya -- payment method
// saldo settle langsung, gak lewat gateway eksternal. Payment method lain TETAP wajib
// gateway_code kayak sebelumnya.
func ResolvePaymentMethod(ctx context.Context, db *bun.DB, paymentMethodID int64, branchID, visitPurposeID int) (*PaymentMethod, error) {
	var pm PaymentMethod
	err := db.NewRaw(`
		SELECT mpm.id, mpm.name, mpm.code, mpm.payment_gateway_code, mpm.payment_method_type_id
		FROM master_payment_method mpm
		WHERE mpm.id = ? AND mpm.is_active = true AND COALESCE(mpm.is_deleted, false) = false
			AND (
				mpm.payment_method_type_id = ?
				OR (mpm.payment_gateway_code IS NOT NULL AND mpm.payment_gateway_code != '')
			)
			AND (
				mpm.flag_all_branch = true
				OR EXISTS (
					SELECT 1 FROM master_payment_method_branches b
					WHERE b.payment_method_id = mpm.id AND b.branch_id = ?
						AND COALESCE(b.is_deleted, false) = false AND b.is_active = true
				)
			)
			AND (
				mpm.flag_all_visitpurpose = true
				OR EXISTS (
					SELECT 1 FROM master_payment_method_visit_purposes vp
					WHERE vp.payment_method_id = mpm.id AND vp.visitpurpose_id = ?
						AND COALESCE(vp.is_deleted, false) = false
				)
			)
	`, paymentMethodID, paymentmethod.WalletPaymentMethodTypeID, branchID, visitPurposeID).Scan(ctx, &pm)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &pm, nil
}

// IsWalletPayment: helper kecil biar pemanggil (order.Create()) gak nulis nil-check +
// perbandingan manual berulang-ulang.
func (pm *PaymentMethod) IsWalletPayment() bool {
	return pm.PaymentMethodTypeID != nil && *pm.PaymentMethodTypeID == paymentmethod.WalletPaymentMethodTypeID
}
