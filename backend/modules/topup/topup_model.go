package topup

import (
	"time"

	"github.com/uptrace/bun"
)

// MemberTopupOnlineModel: mirror tabel member_topup_online (migration 093 di sudocore2) -- TABEL
// YANG SAMA dipakai APIANDORDER buat top-up dari Kiosk/POS (lihat
// APIANDORDER/backend/modules/apipos/membertopup/membertopup_model.go). sudomobile nulis LANGSUNG
// ke tabel ini (connect DB yang sama, gak ada sync/bridge layer), source di sini SELALU 'mobile'.
//
// Baris ini cuma "pemicu" -- begitu status jadi 'paid', baris ke member_balance_ledger baru
// diinsert (saldo member ke-update). Sebelum itu, saldo belum berubah sama sekali.
type MemberTopupOnlineModel struct {
	bun.BaseModel `bun:"table:member_topup_online"`

	ID              int64  `bun:"id,pk,autoincrement"`
	MemberID        int64  `bun:"member_id,notnull"`
	BranchID        *int64 `bun:"branch_id"`
	TerminalID      *int64 `bun:"terminal_id"`
	ReferenceNumber string `bun:"reference_number,notnull"`
	Amount          string `bun:"amount,notnull"` // NUMERIC(20,2) -- string biar presisi gak keganggu float
	Source          string `bun:"source,notnull"` // selalu 'mobile' dari sini
	// PaymentMethodID (migration 221 sudocore2, 2026-09-22) -- kunci UTAMA resolve akun COA di
	// memberbalancejurnal, SELALU keisi (top-up mobile gateway-only). PaymentGatewayCode cuma
	// snapshot (dipakai manggil service payment), BUKAN lagi kunci resolve akun.
	PaymentMethodID    *int64     `bun:"payment_method_id"`
	PaymentGatewayCode *string    `bun:"payment_gateway_code"`
	Status             string     `bun:"status,notnull,default:'pending'"`
	ExpiredAt          *time.Time `bun:"expired_at"`
	CancelAt           *time.Time `bun:"cancel_at"`
	PaidAt             *time.Time `bun:"paid_at"`
	Notes              *string    `bun:"notes"`
	CompanyID          *int       `bun:"company_id"`
	CreatedBy          *int64     `bun:"created_by"`
	CreatedAt          time.Time  `bun:"created_at,notnull,default:current_timestamp"`
}
