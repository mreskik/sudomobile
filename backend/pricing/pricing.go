// Package pricing: logic resolve harga/pajak menu yang dipakai BARENG oleh lebih dari 1 endpoint
// (visit-purpose detail, order/calculate, order/create nanti) -- diekstrak dari
// modules/visitpurpose/visitpurpose_handler.go (2026-08-24) SUPAYA cuma ada 1 sumber kebenaran
// buat perhitungan harga/pajak. JANGAN duplikat logic ini di package lain -- kalau ada endpoint
// baru yang butuh hitung harga/pajak item, import package ini.
package pricing

import (
	"context"
	"fmt"
	"strconv"

	"github.com/uptrace/bun"
)

// VisitPurposeConfig: hasil ResolveVisitPurposeConfig() -- config level VISIT PURPOSE (bukan
// per item): menu_template_id + 3 tax_id (service_charge/vat/pb1, FK ke master_tax) +
// order_fee. service_charge/vat/pb1 nilainya bisa NULL atau 0 -- dua-duanya berarti "gak ada
// pajak jenis itu", sama persis interpretasi POS (MenuServices.php baris 220-241).
type VisitPurposeConfig struct {
	MenuTemplateID int64   `bun:"menu_template_id"`
	ServiceCharge  *int64  `bun:"service_charge"`
	Vat            *int64  `bun:"vat"`
	Pb1            *int64  `bun:"pb1"`
	OrderFee       *string `bun:"order_fee"`
	InclusivePrice bool    `bun:"inclusive_price"`
}

// ResolveVisitPurposeConfig: nil (bukan error) kalau visit purpose-nya gak ketemu/gak cocok
// scope -- pemanggil yang mutusin itu "tidak ditemukan", biar bedain dari error DB beneran.
// Gabung 2 lookup (master_branch_visit_purpose + master_pricelist.inclusive_price) jadi 1
// query lewat JOIN, biar gak nambah round-trip.
func ResolveVisitPurposeConfig(ctx context.Context, db *bun.DB, branchID, visitPurposeID int) (*VisitPurposeConfig, error) {
	var cfg VisitPurposeConfig
	err := db.NewRaw(`
		SELECT bvp.menu_template_id, bvp.service_charge, bvp.vat, bvp.pb1, bvp.order_fee,
			COALESCE(mp.inclusive_price, false) AS inclusive_price
		FROM master_branch_visit_purpose bvp
		LEFT JOIN master_pricelist mp ON mp.id = bvp.menu_template_id
		WHERE bvp.branch_id = ? AND bvp.visit_purpose_id = ? AND bvp.flag_mobile_customer = true AND bvp.is_active = true
	`, branchID, visitPurposeID).Scan(ctx, &cfg)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &cfg, nil
}

// TaxRateMap: tax_id -> rate (string, numeric(10,2) apa adanya dari DB). taxIDOrNil == nil
// ATAU nilainya 0 -> dianggap "gak ada pajak", Rate() balikin nil TANPA nyari ke map (0 emang
// gak pernah di-query ke FetchTaxRates, lihat di bawah).
type TaxRateMap map[int64]string

func (m TaxRateMap) Rate(taxID *int64) *string {
	if taxID == nil || *taxID == 0 {
		return nil
	}
	if rate, ok := m[*taxID]; ok {
		return &rate
	}
	return nil
}

// FetchTaxRates: query master_tax SEKALI buat semua tax_id yang relevan (service_charge/vat/pb1
// visit purpose ini), bukan 3x query terpisah. tax_id yang nil/0 disaring duluan, gak ikut
// di-query (0/nil emang bukan id beneran).
func FetchTaxRates(ctx context.Context, db *bun.DB, ids ...*int64) (TaxRateMap, error) {
	uniqueIDs := []int64{}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id == nil || *id == 0 || seen[*id] {
			continue
		}
		seen[*id] = true
		uniqueIDs = append(uniqueIDs, *id)
	}

	result := TaxRateMap{}
	if len(uniqueIDs) == 0 {
		return result, nil
	}

	rows := []struct {
		ID   int64  `bun:"id"`
		Rate string `bun:"rate"`
	}{}
	if err := db.NewRaw(`SELECT id, rate FROM master_tax WHERE id IN (?)`, bun.In(uniqueIDs)).Scan(ctx, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ID] = row.Rate
	}
	return result, nil
}

// ResolveItemTax: replika PERSIS logic MenuServices.php baris 220-241 -- use_tax item WAJIB
// "vat" atau "pb1" persis (case-sensitive), selain itu (kosong, typo, dst) dianggap gak kena
// pajak. tax_id dari visit purpose yang 0/nil juga dianggap gak ada pajak walau use_tax-nya
// cocok.
func ResolveItemTax(useTax string, cfg *VisitPurposeConfig, rates TaxRateMap) (*int64, *string) {
	var taxID *int64
	switch useTax {
	case "vat":
		taxID = cfg.Vat
	case "pb1":
		taxID = cfg.Pb1
	}
	if taxID == nil || *taxID == 0 {
		return nil, nil
	}
	return taxID, rates.Rate(taxID)
}

// PackageSubItem/PackageGroup: TAHAP 3 visit-purpose-detail (2026-08-24) -- sebagian item punya
// "package" (customer wajib/boleh milih sub-item dari 1+ grup, misal grup "VARIAN" pilih 1 dari
// 2 varian rasa). Struktur & nama field SENGAJA niru pola POS (MenuServices.php
// packageList/menuPackageList), snake_case-in doang.
type PackageSubItem struct {
	MenuPackageID int64   `json:"menu_package_id"`
	ItemID        int64   `json:"item_id"`
	ItemName      string  `json:"item_name"`
	Price         string  `json:"price"`
	IconSrc       *string `json:"icon_src"`
	TaxType       string  `json:"tax_type"`
	TaxID         *int64  `json:"tax_id"`
	TaxRate       *string `json:"tax_rate"`
	DefaultItem   bool    `json:"default_item"`
}

type PackageGroup struct {
	PackageID       int64            `json:"package_id"`
	PackageName     string           `json:"package_name"`
	MinQty          int64            `json:"min_qty"`
	MaxQty          int64            `json:"max_qty"`
	MenuPackageList []PackageSubItem `json:"menu_package_list"`
}

// packageRow: hasil mentah query FetchPackages() -- 1 baris = 1 sub-item package, kebawa
// info parent item + group-nya.
type packageRow struct {
	ParentItemID  int64   `bun:"parent_item_id"`
	PackageID     int64   `bun:"package_id"`
	PackageName   string  `bun:"package_name"`
	MinQty        int64   `bun:"min_qty"`
	MaxQty        int64   `bun:"max_qty"`
	MenuPackageID int64   `bun:"menu_package_id"`
	Price         string  `bun:"price"`
	DefaultItem   bool    `bun:"default_item"`
	SubItemID     int64   `bun:"sub_item_id"`
	SubItemName   string  `bun:"sub_item_name"`
	SubIconSrc    *string `bun:"sub_item_icon_src"`
	SubUseTax     string  `bun:"sub_item_use_tax"`
}

// FetchPackages: batch 1 query buat SEMUA item_id sekaligus (bukan per-item, biar gak N+1) --
// balikin map item_id (item UTAMA, bukan sub-item) -> daftar package group-nya. Item yang gak
// punya package sama sekali gak punya entri di map ini (bukan array kosong).
//
// Sub-item package itu SENDIRINYA baris master_item beneran (lewat item_conversion_detail_id),
// jadi pajaknya diresolve PERSIS sama function (ResolveItemTax) yang dipake item utama -- bukan
// diwarisin dari item utama, bukan logic baru. price di sini dari master_item_package_detail.price
// (konvensi ERP: 0 = "termasuk gratis", bukan 0 = ada tambahan biaya) -- KECUALI kalau
// flag_all_menu_template = false DAN ada override buat menu_template (cfg.MenuTemplateID) ini
// di master_item_package_detail_menu_template, baru dipakai harga override-nya (2026-08-26,
// niru PERSIS logic yang udah dipasang di MenuServices::GetMasterMenuList() POS -- CASE di SQL
// di bawah = versi Go dari resolusi yang sama, fallback ke mipd.price kalau flag true ATAU
// gak ketemu override-nya, BUKAN logic baru).
func FetchPackages(ctx context.Context, db *bun.DB, itemIDs []int64, cfg *VisitPurposeConfig, rates TaxRateMap) (map[int64][]PackageGroup, error) {
	result := map[int64][]PackageGroup{}
	if len(itemIDs) == 0 {
		return result, nil
	}

	rows := []packageRow{}
	err := db.NewRaw(`
		SELECT
			mip.item_id AS parent_item_id,
			mipg.id AS package_id, mipg.name AS package_name, mipg.min_qty, mipg.max_qty,
			mipd.id AS menu_package_id,
			CASE WHEN mipd.flag_all_menu_template THEN mipd.price
				ELSE COALESCE(mt.price, mipd.price)
			END AS price,
			mipd.default_item,
			submi.id AS sub_item_id, submi.item_name AS sub_item_name,
			submi.icon_src AS sub_item_icon_src, submi.use_tax AS sub_item_use_tax
		FROM master_item_package mip
		JOIN master_item_package_group mipg ON mipg.item_package_id = mip.id
		JOIN master_item_package_detail mipd ON mipd.package_group_id = mipg.id
		LEFT JOIN master_item_package_detail_menu_template mt
			ON mt.master_item_package_detail_id = mipd.id AND mt.menu_template_id = ?
		JOIN master_item_conversion_detail submicd ON submicd.id = mipd.item_conversion_detail_id
		JOIN master_item submi ON submi.id = submicd.item_id
		WHERE mip.item_id IN (?)
		ORDER BY mip.item_id ASC, mipg.id ASC, mipd.id ASC
	`, cfg.MenuTemplateID, bun.In(itemIDs)).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		taxID, taxRate := ResolveItemTax(row.SubUseTax, cfg, rates)
		subItem := PackageSubItem{
			MenuPackageID: row.MenuPackageID,
			ItemID:        row.SubItemID,
			ItemName:      row.SubItemName,
			Price:         row.Price,
			IconSrc:       row.SubIconSrc,
			TaxType:       row.SubUseTax,
			TaxID:         taxID,
			TaxRate:       taxRate,
			DefaultItem:   row.DefaultItem,
		}

		groups := result[row.ParentItemID]
		groupIdx := len(groups) - 1
		if groupIdx < 0 || groups[groupIdx].PackageID != row.PackageID {
			groups = append(groups, PackageGroup{
				PackageID:       row.PackageID,
				PackageName:     row.PackageName,
				MinQty:          row.MinQty,
				MaxQty:          row.MaxQty,
				MenuPackageList: []PackageSubItem{},
			})
			groupIdx = len(groups) - 1
		}
		groups[groupIdx].MenuPackageList = append(groups[groupIdx].MenuPackageList, subItem)
		result[row.ParentItemID] = groups
	}

	return result, nil
}

// LineCalculation: hasil CalculateLine() -- angka per 1 UNIT (belum dikali qty), string
// 2 desimal (format numeric(20,2) yang dipakai di seluruh DB ini). Pemanggil yang ngali qty
// buat dapetin subtotal baris.
type LineCalculation struct {
	DPP       string `json:"dpp"`
	NetDPP    string `json:"net_dpp"`
	TaxAmount string `json:"tax_amount"`
	Total     string `json:"total"` // net_dpp + tax_amount, PER UNIT
}

// CalculateLine: replika PERSIS OrderServices.php::RecalculateOrderTotals() baris 753-767
// (netPrice/dpp/netDpp/taxAmount) -- urutan standar PPN: pajak dilepas dulu dari harga kalau
// inclusive (dpp) -> diskon dipotong dari dpp (netDpp) -> pajak final dihitung ULANG dari
// netDpp (taxAmount), BUKAN dari harga awal. discountAmount PER UNIT (bukan dikali qty),
// sama persis semantik row.discount_amount di POS.
//
// Sengaja pakai float64 (BUKAN decimal library) -- niru PERSIS cara POS (PHP float biasa)
// ngitung, termasuk potensi floating-point imprecision-nya -- prinsip yang sama kayak
// ResolveItemTax() yang niru PERSIS behavior POS apa adanya, bukan "dibenerin" jadi lebih
// presisi dari aslinya.
func CalculateLine(price string, taxRate *string, flagInclusiveTax bool, discountAmount float64) (LineCalculation, error) {
	priceF, err := strconv.ParseFloat(price, 64)
	if err != nil {
		return LineCalculation{}, fmt.Errorf("price tidak valid: %w", err)
	}

	rateF := 0.0
	if taxRate != nil {
		rateF, err = strconv.ParseFloat(*taxRate, 64)
		if err != nil {
			return LineCalculation{}, fmt.Errorf("tax_rate tidak valid: %w", err)
		}
	}

	dpp := priceF
	if flagInclusiveTax {
		dpp = priceF / (1 + rateF/100)
	}
	netDpp := dpp - discountAmount
	taxAmount := netDpp * (rateF / 100)

	return LineCalculation{
		DPP:       strconv.FormatFloat(dpp, 'f', 2, 64),
		NetDPP:    strconv.FormatFloat(netDpp, 'f', 2, 64),
		TaxAmount: strconv.FormatFloat(taxAmount, 'f', 2, 64),
		Total:     strconv.FormatFloat(netDpp+taxAmount, 'f', 2, 64),
	}, nil
}
