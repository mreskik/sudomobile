package promo

import (
	"context"
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

// PromoListItem: EXPORTED (2026-09-30) -- dipakai bareng GetList() (member app, di bawah) DAN
// QRHandler.GetListPromo() (order/order_qr_promo_handler.go, QR Order publik) lewat
// BuildPromoList(), biar response schema-nya SAMA PERSIS di kedua endpoint, gak ada 2 struct
// yang gampang ke-drift kalau salah satu diubah belakangan tanpa nyadar yang satu lagi.
type PromoListItem struct {
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
	// FlagRequiredMember (2026-09-30) -- true kalau promo ini cuma bisa dipakai member yang
	// login (lihat KETENTUAN PROMO.md). Dibalikin sebagai info biar FE bisa nampilin badge
	// "khusus member" tanpa nebak-nebak dari kombinasi field lain.
	FlagRequiredMember bool `json:"flag_required_member"`
	// FlagAllTiers/Tiers (2026-09-30) -- flag_all_tiers=true berarti promo berlaku SEMUA tier
	// (Tiers kosong []). false berarti dibatasin ke tier tertentu -- Tiers berisi daftar
	// tier_level+name yang di-allow (lihat pricing.FetchPromoTiers()).
	FlagAllTiers bool                      `json:"flag_all_tiers"`
	Tiers        []pricing.PromoTierOption `json:"tiers"`
	// ImageSrc (2026-10-09) -- path/URL gambar promo (master_promo.image_src di sudocore2,
	// sudah ada sejak migration 248, baru sekarang ditembuskan ke sudomobile). Nullable --
	// null/omitempty kalau promo itu belum diisi gambarnya.
	ImageSrc *string `json:"image_src,omitempty"`
}

// BuildPromoList: EXPORTED (2026-09-30) -- inti logic GetList() DIPISAH biar bisa dipanggil
// ULANG dari QRHandler.GetListPromo() (QR Order, publik, guest -- gak ada member_type_id/
// tier_level) TANPA duplikasi ~40 baris query batch (target_ids/tiers/used_today) + assembly
// loop. Caller yang nentuin memberTypeID/tierLevel (member app: dari member yang login; QR
// Order: 0/0 selalu, guest gak punya identitas member) DAN filter tambahan kalau perlu (QR
// Order MEMBUANG promo FlagRequiredMember=true SETELAH BuildPromoList() manggil ini -- guest
// gak mungkin penuhin syarat itu).
func BuildPromoList(ctx context.Context, db *bun.DB, branchID, visitPurposeID int, memberTypeID int64, tierLevel int) ([]PromoListItem, error) {
	promos, err := pricing.ListEligiblePromos(ctx, db, branchID, visitPurposeID, memberTypeID, tierLevel)
	if err != nil {
		return nil, err
	}

	targetIDs, err := pricing.FetchPromoTargetIDs(ctx, db, promos)
	if err != nil {
		return nil, err
	}

	promoTiers, err := pricing.FetchPromoTiers(ctx, db, promos)
	if err != nil {
		return nil, err
	}

	promoIDs := make([]int64, 0, len(promos))
	for _, p := range promos {
		promoIDs = append(promoIDs, p.ID)
	}
	usedToday, err := pricing.FetchPromoUsedTodayBatch(ctx, db, promoIDs)
	if err != nil {
		return nil, err
	}

	list := make([]PromoListItem, 0, len(promos))
	for _, p := range promos {
		targets := targetIDs[p.ID]
		if targets == nil {
			targets = []int64{}
		}
		tiers := promoTiers[p.ID]
		if tiers == nil {
			tiers = []pricing.PromoTierOption{}
		}
		list = append(list, PromoListItem{
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
			FlagRequiredMember:     p.FlagRequiredMember,
			FlagAllTiers:           p.FlagAllTiers,
			Tiers:                  tiers,
			ImageSrc:               p.ImageSrc,
		})
	}

	return list, nil
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
		return c.JSON(res.SetCode(100).SetMessage("invalid branch_id"))
	}
	visitPurposeID, err := strconv.Atoi(c.Params("visit_purpose_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("invalid visit_purpose_id"))
	}

	memberTypeID, tierLevel, err := pricing.FetchMemberPromoAttrs(c.Context(), h.db, memberID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("failed to fetch member data"))
	}

	list, err := BuildPromoList(c.Context(), h.db, branchID, visitPurposeID, memberTypeID, tierLevel)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("failed to fetch promo data"))
	}

	return c.JSON(res.Success().SetData(list))
}
