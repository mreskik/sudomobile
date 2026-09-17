package paymentmethod

import (
	"sudomobile/backend/helpers"
	"sudomobile/backend/modules/qrorder"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

// QRHandler: interface TERPISAH dari Handler (member app) -- QR Order PUBLIK total, gak ada
// middleware.Auth/AppSetting sama sekali, route-nya didaftarin di group beda (router.go), sama
// pola kayak order.QRHandler/visitpurpose.QRHandler.
type QRHandler interface {
	GetList(c fiber.Ctx) error
}

type qrHandler struct {
	db *bun.DB
}

func NewQRHandler(db *bun.DB) QRHandler {
	return &qrHandler{db: db}
}

// GetList: GET /qr-order/payment-method?db_code=&company_code=&branch_code=&visit_purpose_code=
// -- versi QR Order dari GetList() (member app) di atas. REUSE LANGSUNG resolvePaymentMethodList()
// -- fungsi query yang SAMA PERSIS -- cuma beda cara nunjuk branch+visit_purpose (4 kode di
// query, bukan path). Lihat DOKUMENTASI API/QR ORDER/GET PAYMENT METHOD LIST.md.
func (h *qrHandler) GetList(c fiber.Ctx) error {
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

	list, err := resolvePaymentMethodList(ctx, h.db, qrCtx.BranchID, qrCtx.VisitPurposeID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data payment method"))
	}

	return c.JSON(res.Success().SetData(list))
}
