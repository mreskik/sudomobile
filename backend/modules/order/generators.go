package order

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

// generateReferenceNumber: pola bareng buat order_number & payment_number -- prefix + branch_code
// + timestamp(YmdHis) + 2 digit random. branch_code (bukan branch_id) dipilih biar konsisten sama
// pola PENAMAAN yang dipakai POS (OrderServices::GenerateOrderNumber()/PaymentServices::GenerateOrderNumber()).
//
// Random 2 digit ini SEMENTARA (disepakati 2026-08-26) -- entropinya jauh lebih kecil dari 6 hex
// yang tadinya dipakai generateOrderNumber() (justru buat ngatasin resiko tabrakan concurrency
// banyak device di branch yang sama, detik yang sama). Ditandain sebagai open item, bakal
// direvisi bareng (order_number & payment_number sekaligus), BUKAN dianggap aman permanen.
func generateReferenceNumber(prefix, branchCode string) string {
	return prefix + branchCode + time.Now().Format("20060102150405") + fmt.Sprintf("%02d", rand.Intn(100))
}

// generateOrderNumber: "NO" + branch_code + timestamp + 2 digit random. Dipanggil pas order
// dibuat (create-order) -- SEMUA order dapet ini, terlepas nanti kebayar atau enggak.
func generateOrderNumber(branchCode string) string {
	return generateReferenceNumber("NO", branchCode)
}

// generatePaymentNumber: "QR" + branch_code + timestamp + 2 digit random. Dipanggil BELAKANGAN,
// pas payment settlement (finalizeSettledPayment()) -- BUKAN pas order dibuat. Order yang gak
// pernah kebayar (expired/cancel) gak akan pernah punya payment_number.
func generatePaymentNumber(branchCode string) string {
	return generateReferenceNumber("QR", branchCode)
}

// generateULID: dipakai buat mb_order_detail.ulid/mb_order_detail_package.ulid/mb_order_payment.ulid.
// POS pakai Str::ulid() (Laravel, format ULID asli yang lexicographically sortable by time) -- di
// sini pakai UUID v4 biasa (github.com/google/uuid, udah ada di go.mod, gak nambah dependency
// baru) karena sortability-nya emang gak dipakai/gak dibutuhin logic manapun, cuma butuh unik.
func generateULID() string {
	return uuid.NewString()
}
