package pricing

import (
	"context"

	"github.com/uptrace/bun"
)

// PaymentMethod: hasil ResolvePaymentMethod() -- 1 baris master_payment_method yang lolos
// filter yang SAMA PERSIS kayak GET .../payment-method (gateway-only + scoped branch+visit
// purpose, hormatin flag_all_branch/flag_all_visitpurpose) -- lihat
// DOKUMENTASI API/BRANCH/GET PAYMENT METHOD LIST.md.
type PaymentMethod struct {
	ID                 int64  `bun:"id"`
	Name               string `bun:"name"`
	Code               string `bun:"code"`
	PaymentGatewayCode string `bun:"payment_gateway_code"`
}

// ResolvePaymentMethod: nil (bukan error) kalau payment_method_id gak ketemu ATAU gak lolos
// scope (gateway-only, branch, visit_purpose) -- dipakai buat validasi pas save order (client
// gak boleh kirim payment_method_id sembarangan yang gak lolos filter yang sama kayak listing).
func ResolvePaymentMethod(ctx context.Context, db *bun.DB, paymentMethodID int64, branchID, visitPurposeID int) (*PaymentMethod, error) {
	var pm PaymentMethod
	err := db.NewRaw(`
		SELECT mpm.id, mpm.name, mpm.code, mpm.payment_gateway_code
		FROM master_payment_method mpm
		WHERE mpm.id = ? AND mpm.is_active = true AND COALESCE(mpm.is_deleted, false) = false
			AND mpm.payment_gateway_code IS NOT NULL AND mpm.payment_gateway_code != ''
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
	`, paymentMethodID, branchID, visitPurposeID).Scan(ctx, &pm)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &pm, nil
}
