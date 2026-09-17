package order

import (
	"database/sql"
	"errors"

	"sudomobile/backend/helpers"
	"sudomobile/backend/modules/qrorder"

	"github.com/gofiber/fiber/v3"
)

// qrOrderDetailHeader: versi QR Order dari orderDetailHeader (order_detail_handler.go) -- query &
// struct TERPISAH SENGAJA (bukan nambahin field QR ke orderDetailHeader), pola yang sama kayak
// qrOrderOwnerRow (order_qr_payment_status_handler.go): field set-nya emang beda cukup jauh --
// order_source+branch_id buat cek kepemilikan (bukan member_id/token), order_name+branch
// address buat struk (member app gak butuh, customer-nya login sendiri lewat app). Gak ada
// member_id sama sekali di sini -- order QR Order SELALU tamu.
//
// order_name (BUKAN customer_name, migration sudocore2 211, 2026-09-17) -- disamain sama nama
// kolom yang UDAH ADA di POS, tr_order.order_name.
type qrOrderDetailHeader struct {
	OrderSource         string  `bun:"order_source"`
	OrderNumber         string  `bun:"order_number"`
	Status              string  `bun:"status"`
	CreatedAt           string  `bun:"created_at"`
	SubTotal            string  `bun:"sub_total"`
	TotalDiscount       string  `bun:"total_discount"`
	TotalTax            string  `bun:"total_tax"`
	TotalBilling        string  `bun:"total_billing"`
	FlagInclusiveTax    bool    `bun:"flag_inclusive_tax"`
	OrderName           *string `bun:"order_name"`
	CustomerPhoneNumber *string `bun:"customer_phone_number"`
	BranchID            int     `bun:"branch_id"`
	BranchName          *string `bun:"branch_name"`
	BranchAddress       *string `bun:"branch_address"`
	VisitPurposeID      int64   `bun:"visit_purpose_id"`
	VisitPurposeName    *string `bun:"visit_purpose_name"`
}

// belongsToQRBranch: SAMA PERSIS semantik qrOrderOwnerRow.belongsToQRBranch()
// (order_qr_payment_status_handler.go) -- order member app (order_source='mobile') atau order QR
// branch lain otomatis false, gak ada jalan "nyasar" liat struk order yang bukan haknya.
func (o qrOrderDetailHeader) belongsToQRBranch(branchID int) bool {
	return o.OrderSource == "qr" && o.BranchID == branchID
}

// qrOrderDetailResult: orderDetailResult (member app) MINUS member_id (emang gak pernah ada di
// response member app juga) PLUS order_source/order_name/customer_phone_number/branch_address --
// sesuai spek ORDER DETAIL.md. Gak ada table_number (dibuang dari scope Create Order v1).
type qrOrderDetailResult struct {
	OrderNumber         string             `json:"order_number"`
	Status              string             `json:"status"`
	CreatedAt           string             `json:"created_at"`
	OrderSource         string             `json:"order_source"`
	BranchID            int                `json:"branch_id"`
	BranchName          *string            `json:"branch_name"`
	BranchAddress       *string            `json:"branch_address"`
	VisitPurposeID      int64              `json:"visit_purpose_id"`
	VisitPurposeName    *string            `json:"visit_purpose_name"`
	OrderName           *string            `json:"order_name"`
	CustomerPhoneNumber *string            `json:"customer_phone_number"`
	FlagInclusiveTax    bool               `json:"flag_inclusive_tax"`
	SubTotal            string             `json:"sub_total"`
	TotalDiscount       string             `json:"total_discount"`
	TotalTax            string             `json:"total_tax"`
	TotalBilling        string             `json:"total_billing"`
	Items               []orderDetailItem  `json:"items"`
	Payment             orderDetailPayment `json:"payment"`
}

// GetDetail: GET /qr-order/order/:order_number?db_code=&company_code=&branch_code=&
// visit_purpose_code= -- versi QR Order dari GetDetail() (member app, order_detail_handler.go) di
// atas. REUSE LANGSUNG resolveOrderDetailCore() (item+package+payment, fungsi SAMA PERSIS dipakai
// member app) -- cuma header query & cara ngecek kepemilikan yang beda. Lihat DOKUMENTASI API/QR
// ORDER/ORDER DETAIL.md.
func (h *qrHandler) GetDetail(c fiber.Ctx) error {
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

	var header qrOrderDetailHeader
	err = h.db.NewRaw(`
		SELECT mo.order_source, mo.order_number, mo.status, mo.created_at, mo.sub_total, mo.total_discount,
			mo.total_tax, mo.total_billing, mo.flag_inclusive_tax, mo.order_name, mo.customer_phone_number,
			mo.branch_id, COALESCE(NULLIF(mb.name_qr_order, ''), mb.name) AS branch_name, mb.address AS branch_address,
			mo.visit_purpose_id, mvp.name AS visit_purpose_name
		FROM mb_order mo
		LEFT JOIN master_branch mb ON mb.id = mo.branch_id
		LEFT JOIN master_visit_purpose mvp ON mvp.id = mo.visit_purpose_id
		WHERE mo.order_number = ?
	`, orderNumber).Scan(ctx, &header)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(res.SetCode(100).SetMessage("order tidak ditemukan"))
		}
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data order"))
	}
	if !header.belongsToQRBranch(qrCtx.BranchID) {
		return c.JSON(res.SetCode(100).SetMessage("order tidak ditemukan"))
	}

	items, payment, err := resolveOrderDetailCore(ctx, h.db, orderNumber, header.Status)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data item order"))
	}

	return c.JSON(res.Success().SetData(qrOrderDetailResult{
		OrderNumber:         header.OrderNumber,
		Status:              header.Status,
		CreatedAt:           header.CreatedAt,
		OrderSource:         header.OrderSource,
		BranchID:            header.BranchID,
		BranchName:          header.BranchName,
		BranchAddress:       header.BranchAddress,
		VisitPurposeID:      header.VisitPurposeID,
		VisitPurposeName:    header.VisitPurposeName,
		OrderName:           header.OrderName,
		CustomerPhoneNumber: header.CustomerPhoneNumber,
		FlagInclusiveTax:    header.FlagInclusiveTax,
		SubTotal:            header.SubTotal,
		TotalDiscount:       header.TotalDiscount,
		TotalTax:            header.TotalTax,
		TotalBilling:        header.TotalBilling,
		Items:               items,
		Payment:             payment,
	}))
}
