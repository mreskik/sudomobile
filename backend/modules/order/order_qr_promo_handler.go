package order

import (
	"sudomobile/backend/helpers"
	"sudomobile/backend/modules/promo"
	"sudomobile/backend/modules/qrorder"

	"github.com/gofiber/fiber/v3"
)

// GetListPromo: GET /qr-order/promo?db_code=&company_code=&branch_code=&visit_purpose_code= --
// versi QR Order dari GetList() (member app, modules/promo/promo_handler.go). REUSE
// promo.BuildPromoList() (fungsi inti yang SAMA PERSIS dipakai member app) dengan
// memberTypeID=0/tierLevel=0 (QR Order = tamu, gak ada member_type_id/tier_level SAMA SEKALI --
// pola sama kayak calculateOrder() dipanggil dengan memberID=0).
//
// Filter TAMBAHAN (2026-09-30) yang gak ada di versi member app: promo dengan
// FlagRequiredMember=true DIBUANG dari hasil -- guest QR Order gak mungkin penuhin syarat itu
// (gak ada member_id sama sekali buat dikirim ke Create()), jadi nampilinnya di list ini cuma
// bakal bikin bingung customer milih promo yang ujung-ujungnya ditolak pas coba dipakai.
//
// Response schema SAMA PERSIS GET List Promo member app (promo.PromoListItem) -- FE bisa reuse
// parsing/rendering yang sama antara 2 endpoint ini.
func (h *qrHandler) GetListPromo(c fiber.Ctx) error {
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

	list, err := promo.BuildPromoList(ctx, h.db, qrCtx.BranchID, qrCtx.VisitPurposeID, 0, 0)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data promo"))
	}

	publicList := make([]promo.PromoListItem, 0, len(list))
	for _, p := range list {
		if p.FlagRequiredMember {
			continue
		}
		publicList = append(publicList, p)
	}

	return c.JSON(res.Success().SetData(publicList))
}
