# Account - Top Up Create

Mulai percobaan top-up saldo dompet member yang lagi login. **PROTECTED** (wajib `Authorization: Bearer <token>`, beda dari `/order` yang sekarang publik — lihat [`CREATE ORDER.md`](../../ORDER/CREATE%20ORDER.md)) — gak ada skenario top-up tanpa identitas, saldo yang nambah harus jelas kepunyaan siapa.

**Referensi/arsitektur**: reuse tabel `member_topup_online`/`member_balance_ledger` yang **SAMA PERSIS** dipakai top-up dari Kiosk/POS (lihat `APIANDORDER/backend/modules/apipos/membertopup/`, DTO-nya udah nyiapin `source: "mobile"` dari awal). `sudomobile` nulis LANGSUNG ke tabel itu (connect DB `sudocore2` yang sama, gak ada sync/bridge layer) — **BUKAN** proxy call ke APIANDORDER (endpoint APIANDORDER-nya pakai `BranchTokenAuth`/token device, bukan token member, gak cocok dipanggil dari app customer).

Selalu lewat **payment gateway** — gak ada jalur tunai kayak Kiosk/POS (`payment_gateway_code` kosong = tunai di situ), customer app gak pernah pegang uang fisik.

Lanjutannya: [`CHECK STATUS.md`](CHECK%20STATUS.md) (polling QR abis manggil endpoint ini), [`HISTORY.md`](HISTORY.md) (riwayat semua percobaan).

```
POST /api/account/balance/topup
Authorization: Bearer <token>
```

## Request

```json
{
  "branch_id": 51,
  "amount": "100000",
  "payment_method_id": 1,
  "notes": null
}
```

- `branch_id` — **wajib**. Halaman Top Up Saldo itu bagian dari menu Akun (bukan lagi konteks "lagi pesen di branch X"), jadi gak otomatis ada branch di konteksnya — klien **wajib** suruh user pilih outlet dulu (reuse `GET /api/branch`, sama pola pilih outlet buat order) sebelum submit top-up. Nilai ini disimpen ke `member_topup_online.branch_id` (murni tracking asal transaksi — **BUKAN** dipakai buat jurnal akuntansi, lihat catatan branch jurnal di bawah).
- `amount` — **string**, wajib, `> 0` (2026-09-22, DIBENERIN — sebelumnya sempat angka JSON/`float64`, gak konsisten sama standar aplikasi: semua field nominal uang di sudomobile itu string, bukan float — `tax_amount`/`discount_amount` di order, `balance_in`/`balance_after` di ledger, dst. Diparse pakai `shopspring/decimal`, bukan `float64`, biar presisi gak keganggu floating-point — sama library yang dipakai luas di `sudocore2` buat urusan nominal/akuntansi). Dinormalisasi ulang jadi 2 desimal sebelum disimpan (kolom DB `NUMERIC(20,2)`).
- `payment_method_id` — **wajib** (2026-09-22, sebelumnya `payment_gateway_code` — lihat catatan perbaikan di bawah). Didapat dari [`GET PAYMENT METHOD LIST.md`](../../MENU/GET%20PAYMENT%20METHOD%20LIST.md) yang **SUDAH ADA** (`GET /api/branch/:branch_id/visit-purpose/:visit_purpose_id/payment-method`, ambil field `id` — bukan `code`) — **disepakati eksplisit reuse endpoint ini, TIDAK ada endpoint list payment method terpisah buat top-up**, meskipun konsepnya top-up gak punya `visit_purpose_id` sungguhan (klien kirim `visit_purpose_id` apa saja yang valid buat branch itu, cukup buat dapetin daftar gateway-nya). `payment_gateway_code`-nya sendiri di-resolve **server-side** dari `payment_method_id` ini — gak pernah dipercaya dari body client lagi.
- `notes` — opsional.

**Perbaikan 2026-09-22**: request awalnya minta `payment_gateway_code` (string) langsung dari client. Diubah jadi `payment_method_id` (angka, `master_payment_method.id`) setelah ditemukan bug di alur Kiosk yang sama: `payment_gateway_code` **gak unik** (gak ada constraint di `master_payment_method`) — kalau ada >1 payment method beda pakai kode gateway yang sama, `memberbalancejurnal` bisa salah resolve akun COA pas posting jurnal (`WHERE payment_gateway_code = ? LIMIT 1` ambigu). `payment_method_id` (PK) gak punya masalah itu. Lihat migration `221_alter_member_topup_online_add_payment_method_id.sql` (sudocore2) dan [`MEMBER BALANCE JURNAL.md`](../../../../../sudocore2/DOKUMENTASI%20BACKGROUND%20JOB/MEMBER%20BALANCE%20JURNAL.md).

## Response

Sukses (QR berhasil diminta):

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "reference_number": "TUSBE2026092210004571",
    "status": "pending",
    "qr_string": "00020101021226620014COM.GO-JEK...",
    "qr_url": "https://merchants-app.sbx.midtrans.com/v4/qris/gopay/.../qr-code",
    "expired_at": "2026-09-22T10:15:35+07:00"
  }
}
```

`reference_number` — format `"TU"` + `branch_code` + timestamp(`YmdHis`) + 2 digit random, pola SAMA `order_number`/`payment_number` (lihat [`PAYMENT STATUS.md`](../../ORDER/PAYMENT%20STATUS.md#format-order_number-dan-payment_number-2026-08-26) buat penjelasan lengkap & catatan open item soal entropi random 2 digit). Prefix `"TU"` (top-up) beda dari `"NO"`/`"QR"` yang dipakai order.

Saldo **BELUM** berubah di titik ini — `member_balance_ledger` baru diisi begitu payment settlement confirmed (lewat polling status, lihat [`CHECK STATUS.md`](CHECK%20STATUS.md)).

## Validasi

- `branch_id` kosong → `"branch_id wajib diisi"`.
- `amount` `<= 0` → `"amount wajib lebih dari 0"`.
- `payment_method_id` kosong → `"payment_method_id wajib diisi"`.
- `branch_id` gak ketemu → `"branch tidak ditemukan"`.
- `payment_method_id` gak ketemu → `"payment method tidak ditemukan"`.
- `payment_method_id` ketemu tapi gak punya `payment_gateway_code` → `"payment method tidak didukung"`.
- Gagal minta QR ke service `payment` (down/timeout/kode gateway gak valid) → error dari service `payment` di-passthrough sebagai message, `member_topup_online` ditandain `status = 'failed'` (gak nyangkut `pending` palsu).

## Branch jurnal (background job, sama seperti Kiosk/POS)

Setelah `member_balance_ledger` keisi (`transaction_type = 'topup'`, `jurnal_at IS NULL`), background job `memberbalancejurnal` di `sudocore2` (5 menit sekali, lihat [`MEMBER BALANCE JURNAL.md`](../../../../../sudocore2/DOKUMENTASI%20BACKGROUND%20JOB/MEMBER%20BALANCE%20JURNAL.md)) otomatis posting jurnal: **Debit** akun COA sumber dana (resolve dari `member_topup_online.payment_method_id` → `master_payment_method.coa_accout_id` — **2026-09-22, sebelumnya lewat `payment_gateway_code`, diperbaiki karena ambigu**, lihat catatan di atas), **Credit** akun "Member Wallet Payable". **`BranchId` jurnal-nya BUKAN** dari `member_topup_online.branch_id` (yang cuma tracking "top-up dari outlet mana") — melainkan lookup `master_setting_member_saldo_env`, **1 branch resmi TUNGGAL/global** (bukan per-company lagi, migration `220`) tempat SEMUA jurnal top-up saldo member dibukukan, konsisten. Belum ada baris settingnya sama sekali → jurnal ditolak eksplisit, nunggu admin isi setting (baris tetap `jurnal_at IS NULL`, otomatis di-retry sweep berikutnya).

Ini **TIDAK PERLU** ditangani apa pun dari sisi `sudomobile` — begitu `member_balance_ledger` keisi, sisanya otomatis.

## Sumber data / implementasi

- `sudomobile/backend/modules/topup/topup_service.go` — `CreateTopup()`, `resolvePaymentGatewayCode()`, `generateTopupReference()`.
- `sudomobile/backend/modules/topup/topup_handler.go` — `Create()`.
- `sudomobile/backend/modules/topup/topup_gateway_client.go` — HTTP client ke service `payment`, mirror `modules/order/payment_gateway_client.go` (duplikasi sengaja, belum ada shared helper lintas-module).
- Referensi/pola: `APIANDORDER/backend/modules/apipos/membertopup/membertopup_service.go` (`createGatewayTopup()` — logic identik, cuma `source` beda: `'mobile'` di sini vs bisa `'pos'`/`'kiosk'` di situ).

## Status

**Baru dibuat (2026-09-22), belum tervalidasi live** — `go build`/`go vet` bersih, belum dites end-to-end lewat HTTP request beneran (perlu service `payment` jalan + member session token real). Update bagian ini setelah dites.
