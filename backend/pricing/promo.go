package pricing

import (
	"context"
	"fmt"
	"strconv"

	"github.com/uptrace/bun"
)

// Promo: hasil ResolvePromo() -- 1 baris master_promo yang UDAH lolos semua filter eligibility
// (channel/branch/visit_purpose/member_type/hari/jam/periode/is_active), SELAIN target
// (category/subcategory/item -- itu dicek terpisah lewat PromoTargetMatches(), soalnya beda
// promo bisa target beda item per baris order) dan limit pemakaian (PromoUsedToday()).
type Promo struct {
	ID                     int64   `bun:"id"`
	Name                   string  `bun:"name"`
	Code                   string  `bun:"code"`
	Type                   string  `bun:"type"` // rupiah / percent / freeitem
	TypeRupiahAmount       string  `bun:"type_rupiah_amount"`
	TypePercentUseLimit    bool    `bun:"type_percent_use_limit"`
	TypePercentRate        *string `bun:"type_percent_rate"`
	TypePercentLimitAmount string  `bun:"type_percent_limit_amount"`
	TypeFreeitemItemID     *int64  `bun:"type_freeitem_item_id"`
	PromoFor               string  `bun:"promo_for"` // category / subcategory / item
	MinBuyAmount           string  `bun:"min_buy_amount"`
	MinPointAmount         string  `bun:"min_point_amount"`
	ApplyLimitPerDay       *int64  `bun:"apply_limit_per_day"`
	ApplyLimitPerItem      *int64  `bun:"apply_limit_per_item"`
}

// promoSelectColumns/promoEligibilityConditions: dipecah dari query jadi konstanta biar
// ResolvePromo() (1 promo spesifik, dipakai Calculate()) dan ListEligiblePromos() (semua promo
// yang lolos, dipakai GET .../promo) BENERAN pakai SQL yang SAMA PERSIS -- bukan cuma "niru
// mirip-mirip" yang gampang ke-drift kalau salah satu diubah belakangan tanpa nyadar yang satu
// lagi. Placeholder urutan: branch_id, visit_purpose_id, member_type_id (3 param) -- ResolvePromo
// nambahin 1 placeholder promo_id di depan (WHERE mp.id = ? AND <conditions>).
//
// Query mirror PERSIS MasterController::GetPromoList() (POS, mr_promo), channel di-hardcode
// 'mobile_customer' (bukan 'pos'). memberTypeID boleh 0 (member belum kebaca/gak ada) -- WHERE
// ... = 0 sengaja gak akan pernah match baris master_promo_type_members manapun (id asli gak
// ada yang 0), jadi otomatis fallback ke promo yang flag_all_type_members=true doang, sama
// semantiknya kayak POS ngirim NULL.
const promoSelectColumns = `mp.id, mp.name, mp.code, mp.type, mp.type_rupiah_amount, mp.type_percent_use_limit,
	mp.type_percent_rate, mp.type_percent_limit_amount, mp.type_freeitem_item_id,
	mp.promo_for, mp.min_buy_amount, mp.min_point_amount, mp.apply_limit_per_day, mp.apply_limit_per_item`

const promoEligibilityConditions = `mp.is_active = true
	AND mp.period_start <= CURRENT_DATE AND mp.period_end >= CURRENT_DATE
	AND (
		mp.flag_all_branches = true
		OR EXISTS (SELECT 1 FROM master_promo_branches b WHERE b.promo_id = mp.id AND b.branch_id = ?)
	)
	AND (
		mp.flag_all_visit_purposes = true
		OR EXISTS (SELECT 1 FROM master_promo_visit_purposes vp WHERE vp.promo_id = mp.id AND vp.visit_purpose_id = ?)
	)
	AND (
		mp.flag_all_type_members = true
		OR EXISTS (SELECT 1 FROM master_promo_type_members tm WHERE tm.promo_id = mp.id AND tm.type_member_id = ?)
	)
	AND (
		mp.flag_all_days = true
		OR EXISTS (
			SELECT 1 FROM master_promo_days d
			WHERE d.promo_id = mp.id AND d.day = (ARRAY['minggu','senin','selasa','rabu','kamis','jumat','sabtu'])[EXTRACT(DOW FROM now())::int + 1]
		)
	)
	AND (
		mp.flag_all_times = true
		OR EXISTS (
			SELECT 1 FROM master_promo_times t
			WHERE t.promo_id = mp.id AND now()::time BETWEEN t.time_start AND t.time_end
		)
	)
	AND (
		mp.flag_apply_to_all = true
		OR EXISTS (SELECT 1 FROM master_promo_apply_to a WHERE a.promo_id = mp.id AND a.apply_to = 'mobile_customer')
	)`

// ResolvePromo: nil (bukan error) kalau promo_id gak ketemu ATAU gak lolos salah satu filter
// eligibility -- pemanggil yang mutusin itu "promo tidak berlaku", biar bedain dari error DB
// beneran.
func ResolvePromo(ctx context.Context, db *bun.DB, promoID int64, branchID, visitPurposeID int, memberTypeID int64) (*Promo, error) {
	var promo Promo
	query := fmt.Sprintf("SELECT %s FROM master_promo mp WHERE mp.id = ? AND %s", promoSelectColumns, promoEligibilityConditions)
	err := db.NewRaw(query, promoID, branchID, visitPurposeID, memberTypeID).Scan(ctx, &promo)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &promo, nil
}

// ListEligiblePromos: SEMUA promo yang lolos eligibility struktural (barrier #1-8 di
// KETENTUAN PROMO.md -- is_active/periode/channel/branch/visit_purpose/member_type/hari/jam),
// buat 1 branch+visit_purpose+member. Tipe `freeitem` DIBUANG dari list (belum didukung sama
// sekali, nampilin promo yang bakal ditolak kalau dipakai cuma bikin bingung).
//
// SENGAJA gak filter berdasar min_buy_amount/min_point_amount/apply_limit_per_day di sini --
// itu semua dibalikin sebagai info mentah (dipakai FE nampilin syarat/status), bukan dijadiin
// filter list, karena: (a) min_buy_amount butuh tau subtotal cart yang endpoint LIST ini gak
// punya konteksnya, (b) biar konsisten -- List() nunjukin "promo apa aja yang ADA", Calculate()
// yang jadi gerbang otoritatif final "boleh dipakai apa enggak". Sama persis filosofi POS
// (GetPromoList() juga gak filter itu, cuma nampilin used_today sebagai info).
func ListEligiblePromos(ctx context.Context, db *bun.DB, branchID, visitPurposeID int, memberTypeID int64) ([]Promo, error) {
	promos := []Promo{}
	query := fmt.Sprintf(`
		SELECT %s FROM master_promo mp
		WHERE %s AND mp.type != 'freeitem'
		ORDER BY mp.name ASC
	`, promoSelectColumns, promoEligibilityConditions)
	err := db.NewRaw(query, branchID, visitPurposeID, memberTypeID).Scan(ctx, &promos)
	return promos, err
}

// FetchPromoTargetIDs: batch buat SEMUA promo sekaligus (bukan per-promo, biar gak N+1) --
// balikin map promo_id -> target_ids (isinya category_id/sub_category_id/item_id tergantung
// promo_for punya promo itu MASING-MASING, bisa beda-beda per promo dalam 1 batch). Mirror
// AttachPromoTargetIds() di POS (MasterController.php).
func FetchPromoTargetIDs(ctx context.Context, db *bun.DB, promos []Promo) (map[int64][]int64, error) {
	result := map[int64][]int64{}
	if len(promos) == 0 {
		return result, nil
	}

	var categoryPromoIDs, subcategoryPromoIDs, itemPromoIDs []int64
	for _, p := range promos {
		switch p.PromoFor {
		case "category":
			categoryPromoIDs = append(categoryPromoIDs, p.ID)
		case "subcategory":
			subcategoryPromoIDs = append(subcategoryPromoIDs, p.ID)
		case "item":
			itemPromoIDs = append(itemPromoIDs, p.ID)
		}
	}

	fetch := func(ids []int64, table, column string) error {
		if len(ids) == 0 {
			return nil
		}
		rows := []struct {
			PromoID  int64 `bun:"promo_id"`
			TargetID int64 `bun:"target_id"`
		}{}
		query := fmt.Sprintf(`SELECT promo_id, %s AS target_id FROM %s WHERE promo_id IN (?)`, column, table)
		if err := db.NewRaw(query, bun.In(ids)).Scan(ctx, &rows); err != nil {
			return err
		}
		for _, row := range rows {
			result[row.PromoID] = append(result[row.PromoID], row.TargetID)
		}
		return nil
	}

	if err := fetch(categoryPromoIDs, "master_promo_categories", "category_id"); err != nil {
		return nil, err
	}
	if err := fetch(subcategoryPromoIDs, "master_promo_sub_categories", "sub_category_id"); err != nil {
		return nil, err
	}
	if err := fetch(itemPromoIDs, "master_promo_items", "item_id"); err != nil {
		return nil, err
	}
	return result, nil
}

// FetchPromoUsedTodayBatch: versi batch PromoUsedToday() buat banyak promo sekaligus (dipakai
// List(), biar gak N+1) -- promo yang gak punya entri di map hasilnya berarti belum pernah
// dipakai hari ini (used_today = 0), BUKAN error.
func FetchPromoUsedTodayBatch(ctx context.Context, db *bun.DB, promoIDs []int64) (map[int64]int64, error) {
	result := map[int64]int64{}
	if len(promoIDs) == 0 {
		return result, nil
	}

	rows := []struct {
		PromoID   int64 `bun:"promo_id"`
		UsedToday int64 `bun:"used_today"`
	}{}
	err := db.NewRaw(`
		SELECT mod.promo_id, COUNT(DISTINCT mod.order_number) AS used_today
		FROM mb_order_detail mod
		JOIN mb_order mo ON mo.order_number = mod.order_number
		WHERE mod.promo_id IN (?) AND mo.created_at::date = CURRENT_DATE AND mo.status IN ('pending', 'paid')
		GROUP BY mod.promo_id
	`, bun.In(promoIDs)).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.PromoID] = row.UsedToday
	}
	return result, nil
}

// FetchMemberTypeID: helper kecil, dipakai List() (cuma butuh member_type_id doang, gak perlu
// saldo poin kayak fetchMemberPromoContext() di modules/order -- List gak filter min_point_amount,
// lihat catatan di ListEligiblePromos()).
func FetchMemberTypeID(ctx context.Context, db *bun.DB, memberID int64) (int64, error) {
	var memberTypeID int64
	err := db.NewRaw(`SELECT COALESCE(member_type_id, 0) FROM master_member WHERE id = ?`, memberID).Scan(ctx, &memberTypeID)
	return memberTypeID, err
}

// PromoTargetMatches: promo_for nentuin tabel target mana yang dicek -- category/subcategory
// harus ada nilainya (item tanpa subcategory otomatis gak match promo bertarget subcategory).
func PromoTargetMatches(ctx context.Context, db *bun.DB, promoID int64, promoFor string, itemID int64, categoryID, subcategoryID *int64) (bool, error) {
	var table, column string
	var value int64
	switch promoFor {
	case "item":
		table, column, value = "master_promo_items", "item_id", itemID
	case "category":
		if categoryID == nil {
			return false, nil
		}
		table, column, value = "master_promo_categories", "category_id", *categoryID
	case "subcategory":
		if subcategoryID == nil {
			return false, nil
		}
		table, column, value = "master_promo_sub_categories", "sub_category_id", *subcategoryID
	default:
		return false, nil
	}

	var exists bool
	err := db.NewRaw(fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE promo_id = ? AND %s = ?)`, table, column), promoID, value).Scan(ctx, &exists)
	return exists, err
}

// PromoUsedToday: berapa kali promo ini udah dipakai HARI INI, dihitung dari mb_order_detail
// (order mobile customer doang). SENGAJA gak ngitung/gabung sama pemakaian promo yang sama di
// sisi POS (pos_order_detail) -- scope promo di sudomobile dijaga terpisah dari POS SECARA
// SADAR (2026-08-24, konfirmasi eksplisit): master data promo-nya boleh sama (`master_promo`
// di ERP), tapi limit/kalkulasi pemakaian masing-masing channel independen, dan POS SAMA
// SEKALI gak perlu ngitung ulang apa pun terkait order dari mobile. Ini bukan gap yang
// "kelupaan digabung", ini emang batasan scope yang dipilih.
func PromoUsedToday(ctx context.Context, db *bun.DB, promoID int64) (int64, error) {
	var count int64
	err := db.NewRaw(`
		SELECT COUNT(DISTINCT mod.order_number)
		FROM mb_order_detail mod
		JOIN mb_order mo ON mo.order_number = mod.order_number
		WHERE mod.promo_id = ? AND mo.created_at::date = CURRENT_DATE AND mo.status IN ('pending', 'paid')
	`, promoID).Scan(ctx, &count)
	return count, err
}

// CalculatePromoDiscount: hitung discount_amount PER UNIT dari 1 promo, buat dipassing ke
// CalculateLine(). BEDA dari resolusi harga/pajak lain di package ini (yang niru PERSIS logic
// POS) -- ini logic BARU, karena POS sendiri gak pernah ngitung discount di backend (dihitung
// di frontend, backend cuma nyimpen angka jadi -- lihat riset 2026-08-24). type=freeitem
// SENGAJA belum didukung (mekanismenya beda total, nambah baris item gratis bukan discount di
// baris existing) -- balikin error biar item itu ditolak, bukan diam-diam di-skip.
//
// Percent dihitung dari DPP (harga net-of-tax), konsisten sama titik "diskon dipotong dari dpp"
// di formula DPP-first (CalculateLine). Hasil discount DICLAMP max sebesar dpp itu sendiri
// (gak boleh bikin net_dpp negatif) -- safety yang sengaja ditambah karena ini logic baru, beda
// dari filosofi "niru bug POS apa adanya" yang dipakai buat ResolveItemTax dkk.
func CalculatePromoDiscount(promo *Promo, dpp float64) (float64, error) {
	var discount float64
	switch promo.Type {
	case "rupiah":
		amt, err := strconv.ParseFloat(promo.TypeRupiahAmount, 64)
		if err != nil {
			return 0, fmt.Errorf("type_rupiah_amount promo tidak valid: %w", err)
		}
		discount = amt
	case "percent":
		if promo.TypePercentRate == nil {
			return 0, fmt.Errorf("type_percent_rate promo kosong")
		}
		rate, err := strconv.ParseFloat(*promo.TypePercentRate, 64)
		if err != nil {
			return 0, fmt.Errorf("type_percent_rate promo tidak valid: %w", err)
		}
		discount = dpp * rate / 100
		if promo.TypePercentUseLimit {
			limit, err := strconv.ParseFloat(promo.TypePercentLimitAmount, 64)
			if err != nil {
				return 0, fmt.Errorf("type_percent_limit_amount promo tidak valid: %w", err)
			}
			if discount > limit {
				discount = limit
			}
		}
	case "freeitem":
		return 0, fmt.Errorf("tipe promo freeitem belum didukung")
	default:
		return 0, fmt.Errorf("tipe promo tidak dikenal")
	}

	if discount > dpp {
		discount = dpp
	}
	if discount < 0 {
		discount = 0
	}
	return discount, nil
}
