package topup

// createTopupRequest -- body POST /account/balance/topup. branch_id WAJIB (dikirim dari
// frontend, hasil user pilih outlet di halaman top-up -- lihat diskusi di TOP UP.md kenapa gak
// ada default/auto-resolve) dan sekaligus dipakai isi member_topup_online.branch_id (murni
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
type createTopupRequest struct {
	BranchID        int64   `json:"branch_id"`
	Amount          float64 `json:"amount"`
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
