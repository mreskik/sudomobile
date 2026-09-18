package visitpurpose

import (
	"sudomobile/backend/helpers"
	"sudomobile/backend/modules/qrorder"

	"github.com/gofiber/fiber/v3"
)

// qrVisitPurposeListItem: versi QR Order dari visitPurposeListItem (visitpurpose_handler.go) --
// beda field set SENGAJA: `id` di sini = visit_purpose_id LANGSUNG (bukan id baris
// master_branch_visit_purpose kayak punya member app -- QR Order gak pernah butuh id baris join
// itu, cuma butuh identitas visit purpose-nya doang), DITAMBAH `code` (dipakai jadi
// visit_purpose_code di request berikutnya, satu-satunya cara QR Order nunjuk visit purpose).
type qrVisitPurposeListItem struct {
	ID   int64  `json:"id" bun:"id"`
	Code string `json:"code" bun:"code"`
	Name string `json:"name" bun:"name"`
}

// GetList: GET /qr-order/visit-purpose/list?db_code=&company_code=&branch_code= -- versi QR
// Order dari GetList() (member app, visitpurpose_handler.go) di atas, DI-SCOPE company+branch
// (`qrorder.ResolveBranch()`, 3 kode -- belum butuh visit_purpose_code, endpoint ini justru buat
// NEMUIN visit_purpose_code-nya). Filter PERSIS SAMA (flag_mobile_customer + is_active di kedua
// tabel) -- query ditulis ulang (bukan diekstrak) karena kolom yang di-SELECT beda (`code`
// ditambah, `id` baris bvp dibuang), sama pertimbangan kayak endpoint QR Order lain. Lihat
// DOKUMENTASI API/QR ORDER/ORDER/GET VISIT PURPOSE LIST.md.
func (h *qrHandler) GetList(c fiber.Ctx) error {
	res := helpers.NewResponse()
	ctx := c.Context()

	branchCtx, errMsg, err := qrorder.ResolveBranch(ctx, h.db,
		c.Query("db_code"), c.Query("company_code"), c.Query("branch_code"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal validasi identitas request"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	list := []qrVisitPurposeListItem{}
	if err := h.db.NewRaw(`
		SELECT vp.id, vp.code, vp.name
		FROM master_branch_visit_purpose bvp
		JOIN master_visit_purpose vp ON vp.id = bvp.visit_purpose_id
		WHERE bvp.branch_id = ? AND bvp.flag_mobile_customer = true
			AND bvp.is_active = true AND vp.is_active = true
		ORDER BY vp.name ASC
	`, branchCtx.BranchID).Scan(ctx, &list); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data visit purpose"))
	}

	return c.JSON(res.Success().SetData(list))
}
