package paymentmethod

import (
	"strconv"

	"sudomobile/backend/helpers"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type Handler interface {
	GetList(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

type paymentMethodListItem struct {
	ID         int64   `json:"id" bun:"id"`
	Name       string  `json:"name" bun:"name"`
	Code       string  `json:"code" bun:"code"`
	ColorTheme *string `json:"color_theme" bun:"color_theme"`
}

// GetList: daftar payment method yang bisa dipakai di mobile customer app buat 1
// branch+visit_purpose (2026-08-24). Nested di bawah endpoint visit-purpose-detail
// (`branch/:branch_id/visit-purpose/:visit_purpose_id/payment-method`) karena scoping-nya
// sama persis: branch + visit purpose.
//
// 2 preseden yang beda filosofi di POS:
//   - KioskController::GetPaymentMethodList() -- cuma filter payment_gateway_code gak kosong
//     (Kiosk self-service, gak ada kasir buat mungutin cash), GAK filter branch/visit_purpose
//     sama sekali.
//   - MasterController::GetPaymentMethod() -- filter visit_purpose lewat JOIN doang ke
//     mr_payment_method_visit_purposes, TAPI ini keliatan gak lengkap: gak nangani
//     flag_all_visitpurpose=true (payment method yang berlaku ke SEMUA visit purpose tanpa
//     baris junction) -- kemungkinan gap yang emang ada di POS, bukan sesuatu yang ditiru di
//     sini.
//
// Desain sudomobile (disepakati eksplisit 2026-08-24): filter gateway-only (samain Kiosk --
// mobile customer app itu online-order, gak ada kasir yang mungutin cash), DITAMBAH scoping
// branch+visit_purpose yang bener (flag_all_branch/flag_all_visitpurpose dihormati, mirip pola
// flag_all_brand di master_image_mb_cust) -- karena sudomobile multi-branch beda dari POS yang
// selalu 1 branch per install.
func (h *handler) GetList(c fiber.Ctx) error {
	res := helpers.NewResponse()

	branchID, err := strconv.Atoi(c.Params("branch_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("branch_id tidak valid"))
	}
	visitPurposeID, err := strconv.Atoi(c.Params("visit_purpose_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("visit_purpose_id tidak valid"))
	}

	list := []paymentMethodListItem{}
	err = h.db.NewRaw(`
		SELECT DISTINCT mpm.id, mpm.name, mpm.code, mpm.color_theme
		FROM master_payment_method mpm
		WHERE mpm.is_active = true AND COALESCE(mpm.is_deleted, false) = false
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
		ORDER BY mpm.name ASC
	`, branchID, visitPurposeID).Scan(c.Context(), &list)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data payment method"))
	}

	return c.JSON(res.Success().SetData(list))
}
