// Package qrorder: resolve kode identitas request QR Order (db_code/company_code/branch_code/
// visit_purpose_code) -- SATU tempat, dipakai bareng SEMUA endpoint QR Order biar barrier &
// pesan error-nya konsisten di semua tempat. Lihat DOKUMENTASI API/QR ORDER/KETENTUAN QR
// ORDER.md (bagian "Identitas request" + "Barrier / validasi").
//
// TIGA level resolve (2026-09-18, sebelumnya cuma 1 -- SEMUA endpoint wajib 4 kode penuh):
//   - ResolveCompany() -- db_code+company_code DOANG. Dipakai Get Branch List (customer belum
//     milih branch).
//   - ResolveBranch() -- +branch_code. Dipakai Get Visit Purpose List (customer udah milih
//     branch, belum milih visit purpose).
//   - Resolve() -- +visit_purpose_code (KEEMPAT kode, LENGKAP). Dipakai SEMUA endpoint
//     transaksional/detail (Create, Calculate, Payment Status, Order Detail, Get Visit Purpose
//     Detail, Get Payment Method List) -- endpoint-endpoint ini BUTUH konteks lengkap, gak
//     berubah dari sebelumnya.
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

// CompanyContext: hasil resolve db_code+company_code DOANG (ResolveCompany()).
type CompanyContext struct {
	CompanyID   int
	CompanyCode string
	CompanyName string
}

// BranchContext: hasil resolve +branch_code (ResolveBranch()) -- embed CompanyContext biar field
// company-nya ikut kebawa (qrCtx.CompanyID dst tetep jalan langsung, Go field promotion) tanpa
// duplikasi definisi.
type BranchContext struct {
	CompanyContext

	BranchID      int
	BranchCode    string
	BranchName    string
	BranchAddress *string
}

// Context: hasil resolve LENGKAP, +visit_purpose_code (Resolve()) -- embed BranchContext (yang
// udah embed CompanyContext), jadi SEMUA field lama (CompanyID/BranchID/BranchCode/dst) tetep
// bisa diakses langsung persis kayak sebelum refactor 2026-09-18 ini -- endpoint yang UDAH ADA
// (Create/Calculate/PaymentStatus/OrderDetail/GetVisitPurposeDetail/GetPaymentMethodList) GAK
// PERLU DIUBAH SAMA SEKALI, cuma cara Context ini DIISI di Resolve() yang berubah (lihat bawah).
type Context struct {
	BranchContext

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
	// FlagOnlineServiceMobileCustomer: master_branch_setting.flag_online_service_mobile_customer
	// (2026-09-18) -- flag "QR order aktif" yang tadinya dipertanyakan [BELUM DIPUTUSIN] TERNYATA
	// UDAH ADA (dipakai Get Branch List buat filter tampilan), cuma belum dicek konsisten di
	// endpoint lain -- ditutup sekarang. COALESCE ke false (LEFT JOIN, bukan branch.status yang
	// INNER) -- branch yang gak punya baris master_branch_setting sama sekali dianggap BELUM
	// diaktifin buat mobile/QR, bukan error DB.
	FlagOnlineServiceMobileCustomer bool `bun:"flag_online_service_mobile_customer"`
}

type visitPurposeRow struct {
	ID   int    `bun:"id"`
	Code string `bun:"code"`
	Name string `bun:"name"`
}

// resolveCompanyRow/resolveBranchRow: query MENTAH doang, TANPA cek "required" (pemanggil yang
// cek, masing-masing beda kombinasi field wajib) -- dipakai bareng ResolveCompany/ResolveBranch/
// Resolve biar SATU tempat nulis query-nya, gak triplikasi SQL.
func resolveCompanyRow(ctx context.Context, db *bun.DB, companyCode string) (*companyRow, string, error) {
	company := companyRow{}
	err := db.NewRaw(`SELECT id, code, name FROM master_company WHERE upper(code) = upper(?)`, companyCode).Scan(ctx, &company)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "company tidak ditemukan", nil
		}
		return nil, "", err
	}
	return &company, "", nil
}

// COALESCE name_qr_order -> name -- tampilan khusus QR Order kalau diisi admin, fallback nama
// biasa (sama pola kayak dokumen GET VISIT PURPOSE DETAIL.md). LEFT JOIN master_branch_setting
// (2026-09-18) -- flag_online_service_mobile_customer sekarang ikut jadi gate di sini juga
// (sebelumnya cuma filter tampilan di Get Branch List), SAMA MESSAGE kayak status!='1'
// ("branch tidak aktif") -- sengaja gak dibedain, pola sama kayak "visit purpose tidak
// ditemukan" yang juga nutupin 2 kemungkinan sekaligus (gak perlu bocorin ke client kombinasi
// setting mana yang gagal).
func resolveBranchRow(ctx context.Context, db *bun.DB, branchCode string, companyID int) (*branchRow, string, error) {
	branch := branchRow{}
	err := db.NewRaw(`
		SELECT mb.id, mb.code, COALESCE(NULLIF(mb.name_qr_order, ''), mb.name) AS name, mb.address,
			mb.company_id, mb.status, COALESCE(mbs.flag_online_service_mobile_customer, false) AS flag_online_service_mobile_customer
		FROM master_branch mb
		LEFT JOIN master_branch_setting mbs ON mbs.branch_id = mb.id
		WHERE upper(mb.code) = upper(?)
	`, branchCode).Scan(ctx, &branch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "branch tidak ditemukan", nil
		}
		return nil, "", err
	}
	if branch.CompanyID != companyID {
		return nil, "branch bukan milik company ini", nil
	}
	if branch.Status != "1" {
		return nil, "branch tidak aktif", nil
	}
	if !branch.FlagOnlineServiceMobileCustomer {
		return nil, "branch tidak aktif", nil
	}
	return &branch, "", nil
}

// ResolveCompany: db_code+company_code DOANG -- dipakai Get Branch List (package branch,
// 2026-09-18). Required-check CUMA 2 field ini (branch_code/visit_purpose_code emang gak
// diterima pemanggil ini, jadi gak ada yang perlu dicek).
func ResolveCompany(ctx context.Context, db *bun.DB, dbCode, companyCode string) (*CompanyContext, string, error) {
	if strings.TrimSpace(dbCode) == "" {
		return nil, "db_code wajib diisi", nil
	}
	if strings.TrimSpace(companyCode) == "" {
		return nil, "company_code wajib diisi", nil
	}

	company, errMsg, err := resolveCompanyRow(ctx, db, companyCode)
	if err != nil || errMsg != "" {
		return nil, errMsg, err
	}

	return &CompanyContext{
		CompanyID:   company.ID,
		CompanyCode: company.Code,
		CompanyName: company.Name,
	}, "", nil
}

// ResolveBranch: +branch_code -- dipakai Get Visit Purpose List (package visitpurpose,
// 2026-09-18). SENGAJA gak manggil ResolveCompany() (biar urutan "cek SEMUA required dulu, baru
// mulai query" tetep konsisten dalam lingkup pemanggil ini sendiri -- gak keburu query company
// duluan kalau ternyata branch_code-nya belum diisi; 3 baris required-check ini triplikasi kecil
// sama Resolve() di bawah, sengaja, bukan lupa refactor).
func ResolveBranch(ctx context.Context, db *bun.DB, dbCode, companyCode, branchCode string) (*BranchContext, string, error) {
	if strings.TrimSpace(dbCode) == "" {
		return nil, "db_code wajib diisi", nil
	}
	if strings.TrimSpace(companyCode) == "" {
		return nil, "company_code wajib diisi", nil
	}
	if strings.TrimSpace(branchCode) == "" {
		return nil, "branch_code wajib diisi", nil
	}

	company, errMsg, err := resolveCompanyRow(ctx, db, companyCode)
	if err != nil || errMsg != "" {
		return nil, errMsg, err
	}

	branch, errMsg, err := resolveBranchRow(ctx, db, branchCode, company.ID)
	if err != nil || errMsg != "" {
		return nil, errMsg, err
	}

	return &BranchContext{
		CompanyContext: CompanyContext{
			CompanyID:   company.ID,
			CompanyCode: company.Code,
			CompanyName: company.Name,
		},
		BranchID:      branch.ID,
		BranchCode:    branch.Code,
		BranchName:    branch.Name,
		BranchAddress: branch.Address,
	}, "", nil
}

// Resolve: urutan cek PERSIS tabel barrier di KETENTUAN QR ORDER.md -- berhenti di kegagalan
// PERTAMA, gak lanjut ngecek yang berikutnya:
//  1. Keempat param wajib ada (db_code cuma dicek ADA, isinya belum dipakai/dicocokin ke mana
//     pun -- keputusan sesi 2026-09-17, lihat catatan di KETENTUAN).
//  2. company_code -> master_company (case-insensitive).
//  3. branch_code -> master_branch (case-insensitive), WAJIB company_id-nya cocok #2, WAJIB
//     status aktif ('1'), WAJIB master_branch_setting.flag_online_service_mobile_customer=true
//     (2026-09-18 -- sebelumnya cuma dicek di Get Branch List, sekarang konsisten di sini juga;
//     1 pesan error ("branch tidak aktif") buat DUA kemungkinan, sama pola kayak #4 di bawah).
//  4. visit_purpose_code -> master_visit_purpose (case-insensitive) YANG NYAMBUNG ke branch #3
//     lewat master_branch_visit_purpose (flag_mobile_customer=true, is_active=true) -- 1 query
//     gabungan, 1 pesan error ("visit purpose tidak ditemukan") buat DUA kemungkinan (code
//     gak ada / code ada tapi gak nyambung) -- sengaja gak dibedain, sama semantik kayak
//     endpoint member app.
//
// PERSIS PERILAKU sebelum refactor 2026-09-18 (byte-for-byte urutan cek & pesan error gak
// berubah) -- cuma query company/branch-nya sekarang lewat resolveCompanyRow()/resolveBranchRow()
// yang dipakai bareng ResolveCompany()/ResolveBranch() di atas, bukan ditulis ulang di sini.
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

	company, errMsg, err := resolveCompanyRow(ctx, db, companyCode)
	if err != nil || errMsg != "" {
		return nil, errMsg, err
	}

	branch, errMsg, err := resolveBranchRow(ctx, db, branchCode, company.ID)
	if err != nil || errMsg != "" {
		return nil, errMsg, err
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
		BranchContext: BranchContext{
			CompanyContext: CompanyContext{
				CompanyID:   company.ID,
				CompanyCode: company.Code,
				CompanyName: company.Name,
			},
			BranchID:      branch.ID,
			BranchCode:    branch.Code,
			BranchName:    branch.Name,
			BranchAddress: branch.Address,
		},
		VisitPurposeID:   visitPurpose.ID,
		VisitPurposeCode: visitPurpose.Code,
		VisitPurposeName: visitPurpose.Name,
	}, "", nil
}
