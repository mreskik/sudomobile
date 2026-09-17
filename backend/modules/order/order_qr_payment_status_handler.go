package order

import (
	"database/sql"
	"errors"

	"sudomobile/backend/helpers"
	"sudomobile/backend/modules/qrorder"

	"github.com/gofiber/fiber/v3"
)

// qrOrderOwnerRow: versi QR Order dari orderOwnerRow (order_payment_status_handler.go) -- QR
// Order gak ada konsep "member yang login" sama sekali, jadi "kepemilikan"-nya BUKAN soal
// member_id, tapi order_source='qr' + branch_id cocok sama identitas request (branch_code di
// query, lewat qrorder.Resolve()) -- keputusan sesi 2026-09-17 (KETENTUAN QR ORDER.md, "Akses
// order lama"): SENGAJA cukup ini, TANPA token/no HP tambahan.
type qrOrderOwnerRow struct {
	OrderSource string `bun:"order_source"`
	BranchID    int    `bun:"branch_id"`
	Status      string `bun:"status"`
}

// belongsToQRBranch: true kalau order ini emang order QR DAN branch-nya cocok. Order member app
// (order_source='mobile') otomatis false di sini -- gak ada jalan buat "nyasar" liat status
// pembayaran order member app lewat endpoint QR Order manapun.
func (o qrOrderOwnerRow) belongsToQRBranch(branchID int) bool {
	return o.OrderSource == "qr" && o.BranchID == branchID
}

// PaymentStatus: GET /qr-order/order/:order_number/payment-status?db_code=&company_code=&
// branch_code=&visit_purpose_code= -- versi QR Order dari CheckPaymentStatus() (member app,
// order_payment_status_handler.go) di atas. REUSE LANGSUNG SyncPaymentStatus() (fungsi inti yang
// sama, gak ada logic pembayaran yang ditulis ulang) -- cuma beda cara ngecek "ini boleh diliat
// oleh pemanggil apa enggak" (branch dari 4 kode, bukan member dari token).
func (h *qrHandler) PaymentStatus(c fiber.Ctx) error {
	res := helpers.NewResponse()
	ctx := c.Context()

	qrCtx, errMsg, err := qrorder.Resolve(ctx, h.db,
		c.Query("db_code"), c.Query("company_code"), c.Query("branch_code"), c.Query("visit_purpose_code"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal validasi identitas request"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	orderNumber := c.Params("order_number")

	var order qrOrderOwnerRow
	err = h.db.NewRaw(`SELECT order_source, branch_id, status FROM mb_order WHERE order_number = ?`, orderNumber).Scan(ctx, &order)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("order tidak ditemukan"))
		}
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data order"))
	}
	if !order.belongsToQRBranch(qrCtx.BranchID) {
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
