package order

import (
	"sudomobile/backend/helpers"
	"sudomobile/backend/modules/qrorder"

	"github.com/gofiber/fiber/v3"
)

// qrCalculateRequest: MIRIP calculateRequest (member app) tapi TANPA branch_id/visit_purpose_id
// (dari 4 kode di query, lewat qrorder.Resolve()) -- persis pola qrCreateOrderRequest di
// order_qr_create_handler.go. use_promo_ids REUSE calculateOrder() apa adanya (2026-09-30,
// promo publik dibuka -- lihat catatan di Calculate() di bawah).
type qrCalculateRequest struct {
	Items       []itemRequest `json:"items"`
	UsePromoIDs []int64       `json:"use_promo_ids"`
}

// Calculate: POST /qr-order/calculate?db_code=&company_code=&branch_code=&visit_purpose_code=
// -- versi QR Order dari Calculate() (member app, order_handler.go) di atas. REUSE LANGSUNG
// calculateOrder() -- fungsi inti yang SAMA PERSIS dipakai Create() (member app maupun QR Order)
// -- preview doang, GAK insert apa pun ke mb_order*. Body sama persis [`CREATE ORDER.md`] QR
// Order minus identitas tamu+pembayaran, sengaja dibikin identik biar FE bisa reuse payload yang
// sama antara "hitung dulu" dan "submit beneran" (sama filosofi versi member app).
//
// Promo publik dibuka (2026-09-30, REVISI dari keputusan 2026-09-17 "belum didukung sama
// sekali") -- use_promo_ids diterusin APA ADANYA ke calculateOrder(), TANPA validasi tambahan
// di sini. memberID selalu 0 (QR Order = tamu murni, gak ada login/member_id SAMA SEKALI) --
// calculateOrder() otomatis cuma nerima promo yang FlagRequiredMember=false DAN
// flag_all_type_members=true DAN flag_all_tiers=true (guest gak punya member_type_id/tier_level
// yang valid buat match barrier manapun yang butuh identitas member), lihat KETENTUAN QR
// ORDER.md buat penjelasan lengkap.
func (h *qrHandler) Calculate(c fiber.Ctx) error {
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

	var body qrCalculateRequest
	if err := c.Bind().Body(&body); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body tidak valid"))
	}
	if len(body.Items) == 0 {
		return c.JSON(res.SetCode(100).SetMessage("items tidak boleh kosong"))
	}

	calcReq := calculateRequest{
		BranchID:       qrCtx.BranchID,
		VisitPurposeID: qrCtx.VisitPurposeID,
		Items:          body.Items,
		UsePromoIDs:    body.UsePromoIDs,
	}
	result, errMsg, err := calculateOrder(ctx, h.db, calcReq, 0)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal menghitung order"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	return c.JSON(res.Success().SetData(result))
}
