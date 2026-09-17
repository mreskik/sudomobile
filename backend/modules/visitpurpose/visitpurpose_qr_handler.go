package visitpurpose

import (
	"sudomobile/backend/helpers"
	"sudomobile/backend/modules/qrorder"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type qrCompanyInfo struct {
	ID   int    `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type qrBranchInfo struct {
	ID      int     `json:"id"`
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	Address *string `json:"address"`
}

type qrVisitPurposeInfo struct {
	ID   int    `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// qrVisitPurposeDetail: bentuknya BEDA dari visitPurposeDetail (member app) -- gak ada
// visit_purpose_id di top-level (digantiin blok visit_purpose{} di bawah), DITAMBAH db_code/
// company/branch. Field pajak/menu (MenuTemplateID..Categories) sengaja diduplikasi strukturnya
// (bukan embed visitPurposeDetail) karena bentuk top-level-nya genuinely beda, TAPI ISI-nya
// (Categories, terutama) 100% hasil resolveVisitPurposeDetail() yang sama, cuma dipindah bungkus.
type qrVisitPurposeDetail struct {
	DBCode       string             `json:"db_code"`
	Company      qrCompanyInfo      `json:"company"`
	Branch       qrBranchInfo       `json:"branch"`
	VisitPurpose qrVisitPurposeInfo `json:"visit_purpose"`

	MenuTemplateID    int64          `json:"menu_template_id"`
	FlagInclusiveTax  bool           `json:"flag_inclusive_tax"`
	ServiceCharge     *int64         `json:"service_charge"`
	ServiceChargeRate *string        `json:"service_charge_rate"`
	Vat               *int64         `json:"vat"`
	VatRate           *string        `json:"vat_rate"`
	Pb1               *int64         `json:"pb1"`
	Pb1Rate           *string        `json:"pb1_rate"`
	OrderFee          *string        `json:"order_fee"`
	Categories        []menuCategory `json:"categories"`
}

// QRHandler: interface TERPISAH dari Handler (member app) -- QR Order PUBLIK total, gak ada
// middleware.Auth/AppSetting sama sekali, route-nya didaftarin di group beda (router.go), sama
// pola kayak order.QRHandler.
type QRHandler interface {
	GetDetail(c fiber.Ctx) error
}

type qrHandler struct {
	db *bun.DB
}

func NewQRHandler(db *bun.DB) QRHandler {
	return &qrHandler{db: db}
}

// GetDetail: GET /qr-order/visit-purpose/detail?db_code=&company_code=&branch_code=&
// visit_purpose_code= -- versi QR Order dari GetDetail() (member app) di atas. REUSE LANGSUNG
// resolveVisitPurposeDetail() -- fungsi inti yang SAMA PERSIS (query menu/package/tax, tree
// builder) -- cuma beda cara nunjuk branch+visit_purpose (4 kode di query, bukan path) dan
// bentuk pembungkus response (nambah blok company/branch/visit_purpose hasil resolve, biar FE
// yang cuma megang kode dari QR langsung dapet id+nama). Lihat DOKUMENTASI API/QR ORDER/GET
// VISIT PURPOSE DETAIL.md.
func (h *qrHandler) GetDetail(c fiber.Ctx) error {
	res := helpers.NewResponse()
	ctx := c.Context()

	dbCode := c.Query("db_code")
	qrCtx, errMsg, err := qrorder.Resolve(ctx, h.db,
		dbCode, c.Query("company_code"), c.Query("branch_code"), c.Query("visit_purpose_code"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal validasi identitas request"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	detail, errMsg, err := resolveVisitPurposeDetail(ctx, h.db, qrCtx.BranchID, qrCtx.VisitPurposeID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data visit purpose"))
	}
	if errMsg != "" {
		// Gak akan kejadian secara normal -- qrorder.Resolve() udah mastiin branch+visit_purpose
		// nyambung (flag_mobile_customer+is_active) SEBELUM sampe sini, dijaga tetep konsisten
		// kalau ada race (mis. admin matiin flag PERSIS di antara dua query itu).
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	return c.JSON(res.Success().SetData(qrVisitPurposeDetail{
		DBCode:       dbCode,
		Company:      qrCompanyInfo{ID: qrCtx.CompanyID, Code: qrCtx.CompanyCode, Name: qrCtx.CompanyName},
		Branch:       qrBranchInfo{ID: qrCtx.BranchID, Code: qrCtx.BranchCode, Name: qrCtx.BranchName, Address: qrCtx.BranchAddress},
		VisitPurpose: qrVisitPurposeInfo{ID: qrCtx.VisitPurposeID, Code: qrCtx.VisitPurposeCode, Name: qrCtx.VisitPurposeName},

		MenuTemplateID:    detail.MenuTemplateID,
		FlagInclusiveTax:  detail.FlagInclusiveTax,
		ServiceCharge:     detail.ServiceCharge,
		ServiceChargeRate: detail.ServiceChargeRate,
		Vat:               detail.Vat,
		VatRate:           detail.VatRate,
		Pb1:               detail.Pb1,
		Pb1Rate:           detail.Pb1Rate,
		OrderFee:          detail.OrderFee,
		Categories:        detail.Categories,
	}))
}
