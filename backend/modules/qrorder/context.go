// Package qrorder: resolve 4 kode identitas request QR Order (db_code/company_code/branch_code/
// visit_purpose_code) -- SATU tempat, dipakai bareng SEMUA endpoint QR Order (Create Order +
// Payment Status yang udah jalan, endpoint lain nyusul: payment-method/calculate/order-detail/
// visit-purpose detail) biar barrier & pesan error-nya konsisten di semua tempat. Lihat
// DOKUMENTASI API/QR ORDER/KETENTUAN QR ORDER.md (bagian "Identitas request" + "Barrier /
// validasi").
//
// QR Order PUBLIK total -- gak ada Authorization, gak ada X-App-Setting (beda dari
// middleware.AppSetting yang dipakai member app) -- makanya resolusinya BUKAN Fiber middleware
// yang nulis ke c.Locals (gak ada request lanjutan yang perlu baca locals lintas middleware),
// cukup fungsi biasa yang dipanggil eksplisit di awal tiap handler QR Order.
package qrorder

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/uptrace/bun"
)

// Context: hasil resolve yang sukses -- ID-ID (Company/Branch/VisitPurpose) UDAH divalidasi
// nyambung satu sama lain (branch milik company, visit purpose nyambung ke branch lewat
// master_branch_visit_purpose). Field display (Code/Name/Address) IKUT dibawa sekalian --
// query-nya di Resolve() udah narik baris itu, gak nambah round-trip -- biar endpoint yang
// butuh nge-echo identitas balik (mis. GET VISIT PURPOSE DETAIL, ORDER DETAIL nanti) gak perlu
// query ulang. BranchCode juga dipakai internal (generateOrderNumber() butuh CODE, bukan ID).
type Context struct {
	CompanyID   int
	CompanyCode string
	CompanyName string

	BranchID      int
	BranchCode    string
	BranchName    string
	BranchAddress *string

	VisitPurposeID   int
	VisitPurposeCode string
	VisitPurposeName string
}

type companyRow struct {
	ID   int    `bun:"id"`
	Code string `bun:"code"`
	Name string `bun:"name"`
}

type branchRow struct {
	ID        int     `bun:"id"`
	Code      string  `bun:"code"`
	Name      string  `bun:"name"`
	Address   *string `bun:"address"`
	CompanyID int     `bun:"company_id"`
	Status    string  `bun:"status"`
}

type visitPurposeRow struct {
	ID   int    `bun:"id"`
	Code string `bun:"code"`
	Name string `bun:"name"`
}

// Resolve: urutan cek PERSIS tabel barrier di KETENTUAN QR ORDER.md -- berhenti di kegagalan
// PERTAMA, gak lanjut ngecek yang berikutnya:
//  1. Keempat param wajib ada (db_code cuma dicek ADA, isinya belum dipakai/dicocokin ke mana
//     pun -- keputusan sesi 2026-09-17, lihat catatan di KETENTUAN).
//  2. company_code -> master_company (case-insensitive).
//  3. branch_code -> master_branch (case-insensitive), WAJIB company_id-nya cocok #2, WAJIB
//     status aktif ('1').
//  4. visit_purpose_code -> master_visit_purpose (case-insensitive) YANG NYAMBUNG ke branch #3
//     lewat master_branch_visit_purpose (flag_mobile_customer=true, is_active=true) -- 1 query
//     gabungan, 1 pesan error ("visit purpose tidak ditemukan") buat DUA kemungkinan (code
//     gak ada / code ada tapi gak nyambung) -- sengaja gak dibedain, sama semantik kayak
//     endpoint member app.
//
// Balikin (nil, "pesan error", nil) buat kegagalan validasi/lookup (BUKAN error server -- handler
// tinggal SetMessage() apa adanya), (nil, "", err) buat error DB beneran.
func Resolve(ctx context.Context, db *bun.DB, dbCode, companyCode, branchCode, visitPurposeCode string) (*Context, string, error) {
	if strings.TrimSpace(dbCode) == "" {
		return nil, "db_code wajib diisi", nil
	}
	if strings.TrimSpace(companyCode) == "" {
		return nil, "company_code wajib diisi", nil
	}
	if strings.TrimSpace(branchCode) == "" {
		return nil, "branch_code wajib diisi", nil
	}
	if strings.TrimSpace(visitPurposeCode) == "" {
		return nil, "visit_purpose_code wajib diisi", nil
	}

	company := companyRow{}
	err := db.NewRaw(`SELECT id, code, name FROM master_company WHERE upper(code) = upper(?)`, companyCode).Scan(ctx, &company)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "company tidak ditemukan", nil
		}
		return nil, "", err
	}

	// COALESCE name_qr_order -> name -- tampilan khusus QR Order kalau diisi admin, fallback nama
	// biasa (sama pola kayak dokumen GET VISIT PURPOSE DETAIL.md).
	branch := branchRow{}
	err = db.NewRaw(`
		SELECT id, code, COALESCE(NULLIF(name_qr_order, ''), name) AS name, address, company_id, status
		FROM master_branch WHERE upper(code) = upper(?)
	`, branchCode).Scan(ctx, &branch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "branch tidak ditemukan", nil
		}
		return nil, "", err
	}
	if branch.CompanyID != company.ID {
		return nil, "branch bukan milik company ini", nil
	}
	if branch.Status != "1" {
		return nil, "branch tidak aktif", nil
	}

	visitPurpose := visitPurposeRow{}
	err = db.NewRaw(`
		SELECT mvp.id, mvp.code, mvp.name
		FROM master_visit_purpose mvp
		JOIN master_branch_visit_purpose bvp ON bvp.visit_purpose_id = mvp.id
		WHERE upper(mvp.code) = upper(?) AND bvp.branch_id = ?
			AND bvp.flag_mobile_customer = true AND bvp.is_active = true
	`, visitPurposeCode, branch.ID).Scan(ctx, &visitPurpose)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "visit purpose tidak ditemukan", nil
		}
		return nil, "", err
	}

	return &Context{
		CompanyID:   company.ID,
		CompanyCode: company.Code,
		CompanyName: company.Name,

		BranchID:      branch.ID,
		BranchCode:    branch.Code,
		BranchName:    branch.Name,
		BranchAddress: branch.Address,

		VisitPurposeID:   visitPurpose.ID,
		VisitPurposeCode: visitPurpose.Code,
		VisitPurposeName: visitPurpose.Name,
	}, "", nil
}
