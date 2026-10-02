# Branch - Get Payment Method List

```
GET /api/branch/:branch_id/visit-purpose/:visit_purpose_id/payment-method
```

**Publik, `Authorization` OPSIONAL** (2026-10-01, `middleware.OptionalAuth`) — daftar payment method yang bisa dipakai buat 1 kombinasi branch+visit purpose. Nested di bawah [`GET VISIT PURPOSE DETAIL.md`](GET%20VISIT%20PURPOSE%20DETAIL.md) karena scoping-nya sama persis (branch + visit purpose). Token BOLEH dikirim (opsional) — kalau ada & valid, dipakai buat resolve info saldo member khusus item `WALLET_PAYMENT` (lihat section di bawah).

## Request

`:branch_id` — id branch (dari `GET /api/branch`). `:visit_purpose_id` — FK ke `master_visit_purpose.id` (dari [`GET VISIT PURPOSE LIST.md`](GET%20VISIT%20PURPOSE%20LIST.md)). Header `Authorization: Bearer <token>` **opsional**. Gak ada body.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 1,
      "name": "QRIS",
      "code": "QRA",
      "color_theme": "feaw",
      "payment_type_id": 2,
      "payment_type_name": "CARD"
    }
  ]
}
```

`payment_type_id`/`payment_type_name` — **BARU 2026-10-02**. FK ke `master_payment_method_type` (`id`/`name`, lihat tabel referensinya di bawah) — sebelumnya `payment_method_type_id` sudah dipakai INTERNAL buat deteksi WALLET_PAYMENT (section di bawah) tapi tidak diekspos ke response (`json:"-"`); sekarang diekspos + ditambah nama-nya lewat `LEFT JOIN`. `LEFT JOIN` (bukan `INNER`) karena `payment_method_type_id` nullable di DB — kalau `NULL`, kedua field ini ikut `null`, bukan baris hilang dari list.

Nilai tetap `master_payment_method_type` (dev DB): `1=CASH`, `2=CARD`, `3=VOUCHER`, `4=OTHER`, `5=WALLET_PAYMENT`.

`branch_id`/`visit_purpose_id` yang bukan angka → `{ "code": 100, "message": "branch_id tidak valid" }` / `"visit_purpose_id tidak valid"`. Kombinasi yang gak punya payment method cocok → array kosong `[]`, bukan error.

## WALLET_PAYMENT — `wallet_information` (2026-10-01)

Item yang `master_payment_method.payment_method_type_id = 5` (`WALLET_PAYMENT`, lihat `master_payment_method_type`) dapat field tambahan `wallet_information` — **TIDAK muncul** di item payment method lain (`omitempty`).

Filter gateway-only (`payment_gateway_code IS NOT NULL`) **DILONGGARKAN** khusus item ini — WALLET_PAYMENT **SELALU** muncul di list apa pun isi `payment_gateway_code`-nya (settle langsung dari saldo, bukan lewat gateway eksternal), payment method lain tetap wajib `payment_gateway_code` seperti sebelumnya.

**Ada token valid (member login)** — `wallet_information.code = 0`, berisi `member_name` (`master_member.name`) + `balance` (`member_balance_ledger.balance_after` terbaru, fallback `"0.00"` kalau member belum pernah ada histori saldo). **BARU 2026-10-02**: field ini sebelumnya bernama `saldo`, di-rename jadi `balance` (istilah response diseragamkan ke Inggris) — **breaking change**, frontend yang masih baca `saldo` wajib disesuaikan:

```json
{
  "id": 64,
  "name": "WALLET BALANCE",
  "code": "BCA-001",
  "color_theme": "#ffffff",
  "payment_type_id": 5,
  "payment_type_name": "WALLET_PAYMENT",
  "wallet_information": {
    "code": 0,
    "member_name": "alfathaannn",
    "balance": "17750000.00"
  }
}
```

**Gak ada token valid** — ini mencakup 3 kondisi yang DIPERLAKUKAN SAMA: token gak dikirim sama sekali, token dikirim tapi invalid/expired, DAN **QR Order** (endpoint `/qr-order/payment-method`, lihat `QR ORDER/ORDER/04 GET PAYMENT METHOD LIST.md` — route-nya gak pernah pasang middleware auth apa pun, jadi `member_id` SELALU `0`, WALLET_PAYMENT di situ OTOMATIS SELALU kondisi ini tanpa kode khusus):

```json
{
  "id": 64,
  "name": "WALLET BALANCE",
  "code": "BCA-001",
  "color_theme": "#ffffff",
  "wallet_information": {
    "code": 100,
    "message": "need login"
  }
}
```

**Tervalidasi live (2026-10-01)**: guest (no token) → `need login`. Token valid (`member_id=23`) → `member_name`+`saldo` cocok data DB. Token invalid (string random) → `need login`. QR Order (`/qr-order/payment-method`) → `need login` (selalu, tanpa token apa pun dikirim). QRIS (payment method lain, bukan WALLET_PAYMENT) tidak punya `wallet_information` sama sekali di semua skenario, tidak ada regresi.

**Tervalidasi live `payment_type_id`/`payment_type_name` + rename `balance` (2026-10-02)**: `branch_id=14`/`visit_purpose_id=1`. QRIS → `payment_type_id:2`/`payment_type_name:"CARD"`. WALLET BALANCE → `payment_type_id:5`/`payment_type_name:"WALLET_PAYMENT"`, `wallet_information.balance` muncul benar (bukan `saldo` lagi) baik login maupun guest. QR Order (reuse `resolvePaymentMethodList()`+`enrichWalletInfo()` yang sama persis, tidak ada kode terpisah) otomatis ikut field baru tanpa perubahan kode di `paymentmethod_qr_handler.go`.

Implementasi: `sudomobile/backend/modules/paymentmethod/paymentmethod_handler.go` (`enrichWalletInfo()`, `WalletPaymentMethodTypeID` exported const, `PaymentMethodTypeID`/`PaymentMethodTypeName` sekarang diekspos, `walletInfo.Balance` rename dari `Saldo`), `sudomobile/backend/modules/paymentmethod/paymentmethod_qr_handler.go` (reuse `enrichWalletInfo()` dengan `memberID=0` selalu), `sudomobile/backend/router.go` (`middleware.OptionalAuth` dipasang khusus route ini).

## Analisa & keputusan desain (2026-08-24)

Sebelum dibuat, ditemukan 2 preseden yang beda filosofi di POS buat fitur serupa:

- **`KioskController::GetPaymentMethodList()`** — simpel, cuma filter `payment_gateway_code` gak kosong (Kiosk self-service, gak ada kasir yang bisa mungutin cash/manual). **Gak filter branch/visit_purpose sama sekali.**
- **`MasterController::GetPaymentMethod({visit_purpose_id})`** — filter visit_purpose lewat `JOIN` doang ke tabel junction. **Ketauan gak lengkap**: gak nangani `flag_all_visitpurpose=true` (payment method yang seharusnya berlaku ke SEMUA visit purpose tanpa perlu baris junction) — kemungkinan gap yang emang ada di POS sendiri, bukan sesuatu yang mau ditiru di sini (mirip kasus `service_charge` yang ketemu pas Tahap 2 [`GET VISIT PURPOSE DETAIL.md`](GET%20VISIT%20PURPOSE%20DETAIL.md) — "ada kolomnya tapi gak pernah dipakai bener").

**Keputusan (disepakati eksplisit lewat AskUserQuestion)**: filter gateway-only (samain Kiosk — mobile customer app itu online-order, gak ada kasir), DITAMBAH scoping branch+visit_purpose yang BENER (hormatin `flag_all_branch`/`flag_all_visitpurpose`), karena `sudomobile` ngelayanin banyak branch sekaligus (beda dari POS yang selalu 1 branch per install).

## Sumber data

```sql
SELECT DISTINCT mpm.id, mpm.name, mpm.code, mpm.color_theme,
	mpm.payment_method_type_id, mpmt.name AS payment_method_type_name
FROM master_payment_method mpm
LEFT JOIN master_payment_method_type mpmt ON mpmt.id = mpm.payment_method_type_id
WHERE mpm.is_active = true AND COALESCE(mpm.is_deleted, false) = false
	AND mpm.payment_gateway_code IS NOT NULL AND mpm.payment_gateway_code != ''
	AND (
		mpm.flag_all_branch = true
		OR EXISTS (
			SELECT 1 FROM master_payment_method_branches b
			WHERE b.payment_method_id = mpm.id AND b.branch_id = ?
				AND COALESCE(b.is_deleted, false) = false AND b.is_active = true
		)
	)
	AND (
		mpm.flag_all_visitpurpose = true
		OR EXISTS (
			SELECT 1 FROM master_payment_method_visit_purposes vp
			WHERE vp.payment_method_id = mpm.id AND vp.visitpurpose_id = ?
				AND COALESCE(vp.is_deleted, false) = false
		)
	)
ORDER BY mpm.name ASC
```

Baca langsung dari DB `sudocore2` (`sudomobile` connect ke DB yang sama, gak ada sync/bridge layer kayak POS↔APIANDORDER). Nama tabel junction visit-purpose sengaja dicatat karena gampang salah tebak: **`master_payment_method_visit_purposes`** (plural), bukan `master_payment_method_visit_purpose`.

`COALESCE(is_deleted, false)` dipakai di kedua tabel junction karena kolomnya nullable (pola yang sama kayak `master_pricelist_detail.is_deleted` yang ketemu bug-nya di [`GET VISIT PURPOSE DETAIL.md`](GET%20VISIT%20PURPOSE%20DETAIL.md)) — belum diverifikasi eksplisit bug-nya di tabel ini (gak ada data NULL di dev DB buat dites), tapi dipasang preventif karena polanya identik.

## Tervalidasi live (2026-08-24)

Dites ke data real: cuma 1 payment method di dev DB yang punya `payment_gateway_code` (QRIS, `flag_all_branch=true`, `flag_all_visitpurpose=false`). Baseline test `branch_id=51`/`visit_purpose_id=7` (fixture yang sama dipakai di endpoint menu) → `[]` (benar, junction visit-purpose QRIS cuma ke `visitpurpose_id=1`, bukan `7`). Ditambahin sementara baris junction `(payment_method_id=1, visitpurpose_id=7)` → QRIS langsung muncul, membuktikan resolusi `flag_all_branch=true` (branch manapun lolos) + junction visit-purpose bekerja bareng. Baris test dihapus lagi setelahnya (gak ada data pollution tersisa). Juga dites `branch_id`/`visit_purpose_id` non-angka → pesan error yang sesuai.

Belum sempat dites live: payment method dengan `flag_all_branch=false` (perlu baris di `master_payment_method_branches`) dan `flag_all_visitpurpose=true` — gak ada data existing buat itu di dev DB, tapi logic query-nya simetris sama yang udah kebukti jalan buat sisi visit_purpose.
