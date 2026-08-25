package promo

import (
	"strconv"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"
	"sudomobile/backend/pricing"

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

type promoListItem struct {
	ID                     int64   `json:"id"`
	Name                   string  `json:"name"`
	Code                   string  `json:"code"`
	Type                   string  `json:"type"`
	TypeRupiahAmount       string  `json:"type_rupiah_amount"`
	TypePercentRate        *string `json:"type_percent_rate"`
	TypePercentLimitAmount string  `json:"type_percent_limit_amount"`
	TypePercentUseLimit    bool    `json:"type_percent_use_limit"`
	PromoFor               string  `json:"promo_for"`
	TargetIDs              []int64 `json:"target_ids"`
	MinBuyAmount           string  `json:"min_buy_amount"`
	MinPointAmount         string  `json:"min_point_amount"`
	ApplyLimitPerDay       *int64  `json:"apply_limit_per_day"`
	UsedToday              int64   `json:"used_today"`
}

// GetList: daftar promo yang ELIGIBLE (lolos barrier struktural #1-8 di KETENTUAN PROMO.md --
// is_active/periode/channel mobile_customer/branch/visit_purpose/member_type/hari/jam) buat 1
// branch+visit_purpose+member yang lagi login. PROTECTED, sama alasannya kayak
// order/calculate: filter member_type butuh identitas member.
//
// SENGAJA gak difilter min_buy_amount/min_point_amount/apply_limit_per_day di sini -- list ini
// nunjukin "promo apa aja yang ADA", bukan gerbang final. `Calculate()` (order/calculate) yang
// jadi otoritas terakhir nolak/nerima pas promo BENERAN mau dipakai ke cart -- lihat
// KETENTUAN PROMO.md. Field min_buy_amount/min_point_amount/apply_limit_per_day/used_today
// dibalikin apa adanya sebagai info, biar FE bisa nampilin syarat/status (mis. "min. belanja
// 50rb", "min. 100 poin", "udah kepake hari ini") tanpa nebak-nebak sendiri.
//
// target_ids: isinya category_id/sub_category_id/item_id tergantung promo_for masing-masing
// promo -- FE yang cocokin ke item di cart-nya sendiri buat preview visual (badge "dapat promo"
// misalnya), TAPI keputusan final "kena diskon apa enggak" tetap di server pas Calculate().
func (h *handler) GetList(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	branchID, err := strconv.Atoi(c.Params("branch_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("branch_id tidak valid"))
	}
	visitPurposeID, err := strconv.Atoi(c.Params("visit_purpose_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("visit_purpose_id tidak valid"))
	}

	memberTypeID, err := pricing.FetchMemberTypeID(c.Context(), h.db, memberID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data member"))
	}

	promos, err := pricing.ListEligiblePromos(c.Context(), h.db, branchID, visitPurposeID, memberTypeID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data promo"))
	}

	targetIDs, err := pricing.FetchPromoTargetIDs(c.Context(), h.db, promos)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil target promo"))
	}

	promoIDs := make([]int64, 0, len(promos))
	for _, p := range promos {
		promoIDs = append(promoIDs, p.ID)
	}
	usedToday, err := pricing.FetchPromoUsedTodayBatch(c.Context(), h.db, promoIDs)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data pemakaian promo"))
	}

	list := make([]promoListItem, 0, len(promos))
	for _, p := range promos {
		targets := targetIDs[p.ID]
		if targets == nil {
			targets = []int64{}
		}
		list = append(list, promoListItem{
			ID:                     p.ID,
			Name:                   p.Name,
			Code:                   p.Code,
			Type:                   p.Type,
			TypeRupiahAmount:       p.TypeRupiahAmount,
			TypePercentRate:        p.TypePercentRate,
			TypePercentLimitAmount: p.TypePercentLimitAmount,
			TypePercentUseLimit:    p.TypePercentUseLimit,
			PromoFor:               p.PromoFor,
			TargetIDs:              targets,
			MinBuyAmount:           p.MinBuyAmount,
			MinPointAmount:         p.MinPointAmount,
			ApplyLimitPerDay:       p.ApplyLimitPerDay,
			UsedToday:              usedToday[p.ID],
		})
	}

	return c.JSON(res.Success().SetData(list))
}
