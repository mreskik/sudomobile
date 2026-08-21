package account

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
)

type balanceResponse struct {
	Balance string `json:"balance"`
}

type pointResponse struct {
	Point string `json:"point"`
}

// Balance: saldo TERKINI member yang lagi login -- balance_after baris TERAKHIR di
// member_balance_ledger (BUKAN SUM), sama persis formula GetBalance() di sudocore2
// (backend/modules/master/member/member_services.go). Member yang belum pernah ada transaksi
// saldo sama sekali BUKAN error -- balikin "0.00" apa adanya.
func (h *handler) Balance(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var balance string
	err := h.db.NewRaw(`
		SELECT balance_after FROM member_balance_ledger
		WHERE member_id = ? AND is_deleted = false
		ORDER BY created_at DESC, id DESC LIMIT 1
	`, memberID).Scan(c.Context(), &balance)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("gagal ambil saldo"))
		}
		balance = "0.00"
	}

	return c.JSON(res.Success().SetData(balanceResponse{Balance: balance}))
}

// Point: poin TERKINI member yang lagi login -- sama pola persis kayak Balance(), cuma sumbernya
// member_point_ledger. Belum pernah ada histori poin -- balikin "0".
func (h *handler) Point(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var point string
	err := h.db.NewRaw(`
		SELECT balance_after FROM member_point_ledger
		WHERE member_id = ? AND is_deleted = false
		ORDER BY created_at DESC, id DESC LIMIT 1
	`, memberID).Scan(c.Context(), &point)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("gagal ambil poin"))
		}
		point = "0"
	}

	return c.JSON(res.Success().SetData(pointResponse{Point: point}))
}

type balanceHistoryRow struct {
	ID              int64     `json:"id" bun:"id"`
	TransactionDate time.Time `json:"transaction_date" bun:"transaction_date"`
	TransactionType string    `json:"transaction_type" bun:"transaction_type"`
	Source          string    `json:"source" bun:"source"`
	ReferenceNumber string    `json:"reference_number" bun:"reference_number"`
	BalanceIn       string    `json:"balance_in" bun:"balance_in"`
	BalanceOut      string    `json:"balance_out" bun:"balance_out"`
	BalanceAfter    string    `json:"balance_after" bun:"balance_after"`
	Notes           *string   `json:"notes" bun:"notes"`
}

// BalanceHistory: riwayat transaksi saldo member yang lagi login, terbaru duluan. Query param
// start_date/end_date (format "YYYY-MM-DD"), DUA-DUANYA OPSIONAL -- kosong dua-duanya default
// HARI INI (BUKAN "gak difilter sama sekali"), biar gak narik seluruh histori member tanpa
// sengaja kalau frontend lupa kirim filter. Filter berdasarkan transaction_date, inklusif
// end_date (sampe akhir hari itu). Sama persis aturan/query GetBalanceHistory() di sudocore2.
func (h *handler) BalanceHistory(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	today := time.Now().Format("2006-01-02")
	startDate := strings.TrimSpace(c.Query("start_date"))
	if startDate == "" {
		startDate = today
	}
	endDate := strings.TrimSpace(c.Query("end_date"))
	if endDate == "" {
		endDate = today
	}

	list := []balanceHistoryRow{}
	err := h.db.NewRaw(`
		SELECT id, transaction_date, transaction_type, source, reference_number,
		       balance_in, balance_out, balance_after, notes
		FROM member_balance_ledger
		WHERE member_id = ? AND is_deleted = false
		  AND transaction_date >= ?::date
		  AND transaction_date < (?::date + interval '1 day')
		ORDER BY created_at DESC, id DESC
	`, memberID, startDate, endDate).Scan(c.Context(), &list)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil riwayat saldo"))
	}

	return c.JSON(res.Success().SetData(list))
}

type pointHistoryRow struct {
	ID              int64     `json:"id" bun:"id"`
	TransactionDate time.Time `json:"transaction_date" bun:"transaction_date"`
	TransactionType string    `json:"transaction_type" bun:"transaction_type"`
	ReferenceNumber string    `json:"reference_number" bun:"reference_number"`
	PointConfigName *string   `json:"point_config_name" bun:"point_config_name"`
	PointIn         string    `json:"point_in" bun:"point_in"`
	PointOut        string    `json:"point_out" bun:"point_out"`
	BalanceAfter    string    `json:"balance_after" bun:"balance_after"`
	Notes           *string   `json:"notes" bun:"notes"`
}

// PointHistory: riwayat transaksi poin member yang lagi login, terbaru duluan. Aturan
// start_date/end_date SAMA PERSIS kayak BalanceHistory() (kosong dua-duanya = hari ini).
// point_config_name di-enrich lewat LEFT JOIN master_member_point_config -- null buat baris
// redeem (mmpc_id emang nullable, gak dari config). Sama persis aturan/query GetPointHistory()
// di sudocore2.
func (h *handler) PointHistory(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	today := time.Now().Format("2006-01-02")
	startDate := strings.TrimSpace(c.Query("start_date"))
	if startDate == "" {
		startDate = today
	}
	endDate := strings.TrimSpace(c.Query("end_date"))
	if endDate == "" {
		endDate = today
	}

	list := []pointHistoryRow{}
	err := h.db.NewRaw(`
		SELECT mpl.id, mpl.transaction_date, mpl.transaction_type, mpl.reference_number,
		       mmpc.name as point_config_name,
		       mpl.point_in, mpl.point_out, mpl.balance_after, mpl.notes
		FROM member_point_ledger mpl
		LEFT JOIN master_member_point_config mmpc ON mmpc.id = mpl.mmpc_id
		WHERE mpl.member_id = ? AND mpl.is_deleted = false
		  AND mpl.transaction_date >= ?::date
		  AND mpl.transaction_date < (?::date + interval '1 day')
		ORDER BY mpl.created_at DESC, mpl.id DESC
	`, memberID, startDate, endDate).Scan(c.Context(), &list)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil riwayat poin"))
	}

	return c.JSON(res.Success().SetData(list))
}
