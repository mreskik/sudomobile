package topup

import "time"

// createTopupRequest -- body POST /account/balance/topup. branch_id WAJIB (dikirim dari
// frontend, hasil user pilih outlet di halaman top-up -- lihat diskusi di TOP UP/CREATE.md kenapa
// gak ada default/auto-resolve) dan sekaligus dipakai isi member_topup_online.branch_id (murni
// tracking asal transaksi, TIDAK dipakai buat jurnal -- lihat MEMBER BALANCE JURNAL.md, branch
// jurnal resolve dari master_setting_member_saldo_env, bukan dari sini).
//
// payment_method_id WAJIB -- top-up dari mobile app SELALU lewat payment gateway (gak ada jalur
// tunai kayak Kiosk/POS, customer app gak pernah pegang uang fisik). Klien dapetin id dari
// GET PAYMENT METHOD LIST.md yang SUDAH ADA (reuse, gak ada endpoint list terpisah buat top-up --
// disepakati eksplisit), field `id` di response situ = master_payment_method.id.
//
// payment_gateway_code SENGAJA GAK diterima dari body (2026-09-22, konsisten sama perbaikan
// APIANDORDER/Kiosk) -- di-resolve SERVER-SIDE dari payment_method_id di service.go, BUKAN
// dipercaya dari client. payment_method_id yang tersimpan ke member_topup_online.payment_method_id
// (PK master_payment_method, unik) -- BUKAN payment_gateway_code (gak ada constraint unique di
// situ, bisa ambigu kalau ada >1 payment method beda pakai kode gateway yang sama -- ini bug yang
// sempat kejadian di Kiosk sebelum diperbaiki bareng, lihat DOKUMENTASI BACKGROUND JOB/MEMBER
// BALANCE JURNAL.md di sudocore2).
// Amount string (2026-09-22, DIBENERIN -- sebelumnya sempat float64, gak konsisten sama standar
// aplikasi: SEMUA field nominal uang di sudomobile itu string, bukan float -- TaxAmount/
// DiscountAmount/dst di order, balance_in/balance_after di ledger, dst. float cuma buat data yang
// emang butuh presisi desimal bebas kayak koordinat GPS (latitude/longitude branch), BUKAN duit).
type createTopupRequest struct {
	BranchID        int64   `json:"branch_id"`
	Amount          string  `json:"amount"`
	PaymentMethodID int64   `json:"payment_method_id"`
	Notes           *string `json:"notes"`
}

// createTopupResponse -- top-up mobile SELALU gateway (gak ada jalur tunai/langsung final kayak
// createCashTopup APIANDORDER), jadi status SELALU "pending" + data QR di response sukses.
type createTopupResponse struct {
	ReferenceNumber string  `json:"reference_number"`
	Status          string  `json:"status"`
	QRString        *string `json:"qr_string,omitempty"`
	QRURL           *string `json:"qr_url,omitempty"`
	ExpiredAt       *string `json:"expired_at,omitempty"`
}

// checkTopupStatusResponse -- respons GET check-status. BalanceAfter cuma keisi kalau Status
// udah "paid" (saldo beneran ke-update).
type checkTopupStatusResponse struct {
	ReferenceNumber string  `json:"reference_number"`
	Status          string  `json:"status"`
	BalanceAfter    *string `json:"balance_after,omitempty"`
}

// topupHistoryRow -- 1 baris respons GET history. SEMUA percobaan top-up (pending/paid/expired/
// cancel/failed), BEDA dari BALANCE HISTORY.md (account/balance_handler.go) yang cuma nampilin
// transaksi yang UDAH settlement (baca member_balance_ledger, baris situ baru ada abis paid).
// Di sini baca member_topup_online langsung -- customer bisa liat "topup gue kemarin kenapa gak
// masuk-masuk" (percobaan gagal/expired/masih pending juga kelihatan).
//
// BranchName (2026-09-22) -- JOIN master_branch, resolve dari member_topup_online.branch_id
// (branch ASAL transaksi top-up, murni tracking -- BUKAN branch jurnal, lihat catatan di
// CreateTopup()/MEMBER BALANCE JURNAL.md). Nullable -- branch_id sendiri nullable di skema
// (walau sudomobile SELALU ngisi, kolom ini shared sama Kiosk/POS yang mungkin beda perilaku).
// Source -- 'pos'/'kiosk'/'mobile', biar keliatan asal top-up itu darimana (relevan karena
// history ini gak di-filter source, tapi scoped ke member yang login -- 1 member bisa top-up
// dari channel manapun).
type topupHistoryRow struct {
	ReferenceNumber string     `json:"reference_number"`
	Amount          string     `json:"amount"`
	Status          string     `json:"status"`
	Source          string     `json:"source"`
	BranchName      *string    `json:"branch_name"`
	CreatedAt       time.Time  `json:"created_at"`
	PaidAt          *time.Time `json:"paid_at,omitempty"`
}
