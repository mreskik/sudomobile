package order

import (
	"strings"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
)

type orderHistoryItem struct {
	OrderNumber       string  `json:"order_number" bun:"order_number"`
	Status            string  `json:"status" bun:"status"`
	OrderIn           string  `json:"order_in" bun:"order_in"`
	TotalBilling      string  `json:"total_billing" bun:"total_billing"`
	TotalItem         int64   `json:"total_item" bun:"total_item"`
	BranchID          int64   `json:"branch_id" bun:"branch_id"`
	BranchName        *string `json:"branch_name" bun:"branch_name"`
	VisitPurposeID    int64   `json:"visit_purpose_id" bun:"visit_purpose_id"`
	VisitPurposeName  *string `json:"visit_purpose_name" bun:"visit_purpose_name"`
	PaymentMethodID   *int64  `json:"payment_method_id" bun:"payment_method_id"`
	PaymentMethodName *string `json:"payment_method_name" bun:"payment_method_name"`
	PaymentExpiredAt  *string `json:"payment_expired_at" bun:"payment_expired_at"`
}

// GetHistory: GET /api/order/history -- list HEADER order milik member yang login. Mirror
// KIOSK ORDER HISTORY.md POS, tapi di-scope ke member_id (BUKAN terminal_id -- Kiosk itu 1
// device dipakai gantian banyak kasir/customer, sudomobile 1 akun = 1 customer, jadi scope-nya
// otomatis "punya siapa" bukan "dari device mana").
//
// BEDA dari Kiosk soal default rentang tanggal: Kiosk default ke HARI INI (staff cuma perlu
// liat transaksi shift berjalan), sudomobile default TANPA batas tanggal (customer wajar mau
// liat SEMUA riwayat order-nya, bukan cuma hari ini) -- date_from/date_to di sini OPSIONAL
// murni buat filter tambahan kalau riwayatnya udah panjang.
//
// payment_method_id/payment_method_name/payment_expired_at diambil dari ATTEMPT TERAKHIR
// mb_order_payment_request (bukan mb_order_payment) -- sama alasan kayak Kiosk: biar tetap
// keisi buat order yang masih `pending` (belum kebayar), berguna kalau nanti dibikin fitur
// "lanjutkan bayar" dari list history (belum dibangun -- retry payment sengaja di-skip untuk
// sekarang, lihat CANCEL ORDER.md).
func (h *handler) GetHistory(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	dateFrom := strings.TrimSpace(c.Query("date_from"))
	dateTo := strings.TrimSpace(c.Query("date_to"))

	query := `
		SELECT
			mo.order_number, mo.status, mo.created_at AS order_in, mo.total_billing,
			COALESCE(item_count.total_item, 0) AS total_item,
			mo.branch_id, mb.name AS branch_name,
			mo.visit_purpose_id, mvp.name AS visit_purpose_name,
			latest_pr.payment_method_id, mpm.name AS payment_method_name, latest_pr.expired_at AS payment_expired_at
		FROM mb_order mo
		LEFT JOIN master_branch mb ON mb.id = mo.branch_id
		LEFT JOIN master_visit_purpose mvp ON mvp.id = mo.visit_purpose_id
		LEFT JOIN (
			SELECT order_number, SUM(qty) AS total_item FROM mb_order_detail GROUP BY order_number
		) item_count ON item_count.order_number = mo.order_number
		LEFT JOIN LATERAL (
			SELECT payment_method_id, expired_at FROM mb_order_payment_request
			WHERE order_number = mo.order_number ORDER BY created_at DESC LIMIT 1
		) latest_pr ON true
		LEFT JOIN master_payment_method mpm ON mpm.id = latest_pr.payment_method_id
		WHERE mo.member_id = ?
	`
	args := []any{memberID}

	if dateFrom != "" {
		query += " AND mo.created_at >= ?::date"
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		query += " AND mo.created_at < (?::date + interval '1 day')"
		args = append(args, dateTo)
	}
	query += " ORDER BY mo.created_at DESC"

	list := []orderHistoryItem{}
	if err := h.db.NewRaw(query, args...).Scan(c.Context(), &list); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil riwayat order"))
	}

	return c.JSON(res.Success().SetData(list))
}
