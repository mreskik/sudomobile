# Order - CREATE ORDER

```
POST /api/order/create-order
```

**PUBLIK** (2026-09-22, sebelumnya PROTECTED — `Authorization: Bearer <token>` sekarang OPSIONAL, `X-App-Setting` TETAP wajib) — bikin order beneran (insert `mb_order*`) DAN sekaligus minta QR pembayaran ke service `payment`. 1 call dari sisi client, internal-nya 2 langkah backend (konfirmasi 2026-08-24) — lihat bagian "Alur" di bawah.

Kalau login (ada token valid), order tersimpan dengan `member_id` terisi (muncul di [`ORDER HISTORY.md`](ORDER%20HISTORY.md), dapat point/benefit member kalau ada). Kalau gak login, `mb_order.member_id` disimpan `NULL` (order tamu, sama seperti [QR Order](../../QR%20ORDER/KETENTUAN%20QR%20ORDER.md)) — **`order_number` itu sendiri jadi satu-satunya kunci akses** ke order ini (detail/cancel/payment-status gak lagi wajib login, siapa pun yang tau/pegang nomornya berhak akses, lihat [`ORDER DETAIL.md`](ORDER%20DETAIL.md)). Promo (`use_promo_ids`) **⚠️ REVISI 2026-09-30** — TIDAK lagi wajib login secara blok total, tergantung `master_promo.flag_required_member` per promo (lihat [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md) barrier #6c).

**Bug fix (2026-09-23)**: sejak digeser jadi publik (2026-09-22), grup route `/order/*` sempat KEHILANGAN middleware auth sama sekali (`middleware.Auth` dicopot karena sifatnya hard-reject, gak cocok buat publik, tapi gak ada pengganti yang dipasang) — akibatnya `middleware.MemberID(c)` SELALU balik `0` walau client kirim token valid, jadi `mb_order.member_id` SELALU `NULL` (bahkan pas login). Fix: middleware baru `middleware.OptionalAuth` (`backend/middleware/auth.go`) dipasang ke `orderPublicRouter` (`backend/router.go`) — resolve token KALAU ADA & valid (isi `member_id` ke locals sama kayak `Auth`), tapi gak nolak request kalau token kosong/invalid/expired (beda dari `Auth` yang hard-reject). Sekarang login beneran ngaruh: `member_id` valid → `mb_order.member_id` kesimpen (bukan `NULL`, bukan juga literal `0` — tetap lewat konversi pointer `*int64` yang udah ada di `insertOrder()`).

Body **SAMA PERSIS** kayak [`CALCULATE.md`](CALCULATE.md) DITAMBAH `payment_method_id`/`customer_phone_number`/`customer_name`/`pin` — logic resolve harga/pajak/promo dipakai ULANG persis (fungsi `calculateOrder()` yang sama), jadi breakdown yang tampil pas preview keranjang GAK PERNAH beda sama yang beneran kesimpen/ke-charge.

## Request

```json
{
  "branch_id": 51,
  "visit_purpose_id": 7,
  "payment_method_id": 1,
  "customer_phone_number": "081234567890",
  "customer_name": "Budi Santoso",
  "pin": "123456",
  "use_promo_ids": [23],
  "items": [
    {
      "menu_id": 109,
      "qty": 2,
      "notes": "less ice",
      "packages": [
        {
          "package_id": 18,
          "selections": [{ "menu_package_id": 31, "qty": 1 }]
        }
      ]
    }
  ]
}
```

- `branch_id`/`visit_purpose_id`/`items`/`use_promo_ids` — sama persis [`CALCULATE.md`](CALCULATE.md), lihat dokumen itu buat detail lengkap (termasuk [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md)).
- `payment_method_id` — **wajib**, harus lolos filter yang sama kayak [`GET PAYMENT METHOD LIST.md`](../MENU/GET%20PAYMENT%20METHOD%20LIST.md) (gateway-only, scoped branch+visit_purpose).
- `customer_phone_number` — opsional.
- `customer_name` — **BARU 2026-09-23, opsional**. Keisi ke `mb_order.order_name` (kolom sama yang dipakai [QR Order](../../QR%20ORDER/KETENTUAN%20QR%20ORDER.md), di situ wajib karena satu-satunya identitas tamu). Di member app ini cuma pelengkap — identitas utama tetap `member_id` dari token kalau login. Kosong/gak dikirim → `order_name` disimpan `NULL`. Ikut dibalikin di response [`ORDER DETAIL.md`](ORDER%20DETAIL.md) sebagai `customer_name`.
- `pin` — **BARU 2026-10-02, OPSIONAL untuk payment method lain, TAPI WAJIB & divalidasi kalau `payment_method_id` resolve ke WALLET_PAYMENT** (lihat section di bawah). PIN 6 digit member yang sama dengan [`PIN CREATE.md`](../AUTH/PIN%20CREATE.md)/[`LOGIN PIN.md`](../AUTH/LOGIN%20PIN.md).

## Response

Sukses (payment gateway berhasil diminta):

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "order_number": "NOSBE2026082610004571",
    "status": "pending",
    "sub_total": "111930.00",
    "total_tax": "11193.00",
    "total_discount": "0.00",
    "total_billing": "123123.00",
    "point_redeem_amount": 0,
    "items": [
      /* sama struktur kayak items di CALCULATE.md */
    ],
    "payment": {
      "status": "pending",
      "vendor_qr_string": "00020101021226620014COM.GO-JEK...",
      "vendor_qr_url": "https://merchants-app.sbx.midtrans.com/v4/qris/gopay/.../qr-code",
      "expired_at": "2026-08-25T08:13:35+07:00",
      "failure_reason": null
    }
  }
}
```

Kalau service `payment` gagal dipanggil (down/timeout/error) — **order TETAP kebuat** (`code: 0`, order valid, ada di DB), cuma `payment.status` jadi `"failed"` dan QR kosong:

```json
{
  "payment": {
    "status": "failed",
    "vendor_qr_string": null,
    "vendor_qr_url": null,
    "expired_at": null,
    "failure_reason": "Post \"http://localhost:98/payment-gateway/qris\": dial tcp ...: connection refused"
  }
}
```

Retry payment request buat order yang statusnya `payment.status: failed` **belum ada endpoint terpisahnya** — di luar scope saat ini, dicatat sebagai next step.

## Alur (2 langkah backend, 1 call client)

1. **Hitung & simpan** — `calculateOrder()` (fungsi INTI yang sama dipakai `Calculate()`) resolve+validasi ulang semua (`branch_id`/`visit_purpose_id` → `menu_template_id`, item, package, promo — gak percaya apa pun dari client). Kalau lolos, generate `order_number` (format `"NO" + branch_code + timestamp(YmdHis) + 2 digit random` — lihat [PAYMENT STATUS.md](PAYMENT%20STATUS.md#format-order_number-dan-payment_number-2026-08-26) buat penjelasan lengkap format ini & pasangannya `payment_number`), lalu insert `mb_order` + `mb_order_detail`(+`_package`) dalam **1 transaksi** (rollback total kalau ada yang gagal).
2. **Minta QR** — insert `mb_order_payment_request` (status `pending`) SEBELUM manggil service `payment`, baru `POST {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/qris` (`sudomobile/backend/modules/order/payment_gateway_client.go`, mirror `App\Services\PaymentGatewayServices::RequestPayment()` POS). Gagal → baris di-update `status: failed` (bukan nyangkut `pending` palsu), TAPI order dari langkah 1 gak di-rollback — itu udah order yang valid, cuma belum ada cara bayarnya buat sekarang.

`order_type` di-hardcode `"takeaway"`, `pax` dibiarin `NULL` (keputusan 2026-08-24, mobile gak ada dine-in). `order_fee`/`service_charge`/`platform_fee`/`delivery_cost` di `mb_order` disimpen `0` — **SAMA PERSIS** gap yang udah didokumentasikan di [`GET VISIT PURPOSE DETAIL.md`](../MENU/GET%20VISIT%20PURPOSE%20DETAIL.md) (`service_charge` diresolve tapi gak pernah diterapkan ke perhitungan manapun di seluruh ekosistem ini — bukan hal baru yang kelewat di sini).

### Potong poin buat promo bersyarat poin (`min_point_amount`, 2026-09-30)

Kalau salah satu promo di `use_promo_ids` punya `master_promo.min_point_amount > 0` (lihat [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md)), field itu berperan **ganda**: syarat minimum kepemilikan poin (dicek di `calculateOrder()`, sudah ada) **dan** besaran poin yang beneran DIPOTONG dari saldo member — nilainya sama persis, `min_point_amount` itu sendiri. `point_redeem_amount` di response = SUM `min_point_amount` dari semua promo yang lolos & match ke minimal 1 item (unik per promo, bukan per baris item).

**Poin dipotong LANGSUNG di titik `create` ini** (bukan nunggu payment settlement) — insert baris baru ke `member_point_ledger` (`transaction_type = 'redeem'`, ledger-based, **BUKAN** `UPDATE` kolom saldo) dalam **transaksi yang sama** dengan insert `mb_order`. Alasan potong di depan, bukan di titik pembayaran diterima: kalau barrier-nya ditaruh di settlement, ada risiko uang customer **sudah settle di gateway Midtrans** tapi order gagal diproses gara-gara validasi poin gagal — selisih uang yang harus direkonsiliasi manual. Potong-di-depan menghindari itu total.

**Tidak ada validasi ulang "saldo cukup" di titik potong ini** — kalau ternyata race (2 order dibuat nyaris bersamaan, promo sama, saldo cuma cukup buat 1) bikin saldo jadi negatif, itu **dibiarkan** (bukan digagalkan). Efek sampingnya justru jadi mekanisme reservasi otomatis: order kedua yang mencoba pakai promo sama bakal baca saldo yang sudah berkurang duluan sama order pertama (via `calculateOrder()`), sehingga validasi `min_point_amount` di `create`-nya sendiri yang menolak — tidak butuh locking/reservasi terpisah.

Promo bersyarat poin **wajib login** (`member_id` bukan `NULL`) — konsekuensinya, promo apa pun yang `min_point_amount > 0` WAJIB juga `flag_required_member = true` (ditegakkan keras di sisi admin, lihat [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md) barrier #6c), jadi barrier #6c yang bakal nolak duluan kalau gak login. Diperkuat lagi eksplisit di `redeemMemberPoint()` (`insertOrder()` return error kalau `memberIDParam == nil` padahal `point_redeem_amount > 0` — harusnya gak pernah kejadian normal karena barrier #6c udah cegah duluan, murni pengaman tambahan).

**Refund**: kalau order ini nantinya `expired` (lihat [`PAYMENT STATUS.md`](PAYMENT%20STATUS.md)) atau di-`cancel` (lihat [`CANCEL ORDER.md`](CANCEL%20ORDER.md)) sebelum sempat `paid`, poin yang terpotong di titik `create` ini **dikembalikan otomatis** (insert baris baru `transaction_type = 'redeem_reversal'`, bukan edit/hapus baris lama).

## WALLET_PAYMENT — bayar pakai saldo member (2026-10-01)

`payment_method_id` yang resolve ke `master_payment_method.payment_method_type_id = 5` (`WALLET_PAYMENT`, lihat [`GET PAYMENT METHOD LIST.md`](../MENU/GET%20PAYMENT%20METHOD%20LIST.md)) jalan lewat **jalur TERPISAH TOTAL** dari payment gateway — settle **INSTAN** saat `Create()` ini dipanggil, bukan nunggu QR di-scan/polling.

**Beda dari jalur gateway (QRIS dkk):**

- Order langsung `status: "paid"` di response (bukan `"pending"`) — `payment_number` sudah terisi SAAT ITU JUGA (beda dari gateway yang `payment_number` baru muncul belakangan pas `finalizeSettledPayment()`).
- `mb_order_payment` langsung di-insert di titik ini juga (bukan nunggu settlement) — kolom `deduct_member_id` (migration sudocore2 241) diisi `member_id` yang saldonya dipotong.
- Saldo (`member_balance_ledger`) langsung dipotong — baris baru `transaction_type='payment'`, `source='mobile'`, `reference_number=order_number`.
- `requestPaymentForOrder()`/`mb_order_payment_request` **SAMA SEKALI TIDAK dipanggil/diisi** untuk jalur ini — tidak relevan, tidak ada apa pun yang perlu di-polling.
- `pg_notify('mb_order_paid', ...)` tetap dipanggil (di dalam transaksi yang sama) — POS tetap langsung tahu ada order baru lewat jalur pull yang sama seperti order gateway.

**Barrier khusus** (dicek SEBELUM proses, SETELAH `calculateOrder()`/`ResolvePaymentMethod()` normal):

1. **Wajib login** — `member_id` kosong (gak ada token / token invalid/expired) → `"please login to use wallet payment"`. Guest/QR Order tidak bisa pakai WALLET_PAYMENT sama sekali (sejalan dengan [`GET PAYMENT METHOD LIST.md`](../MENU/GET%20PAYMENT%20METHOD%20LIST.md) yang juga selalu `need login` di situasi yang sama).
2. **PIN wajib & benar (BARU 2026-10-02)** — dicek SETELAH login, SEBELUM saldo (gagal di hal yang lebih murah dulu). `pin` kosong → `"pin is required for wallet payment"`. Diisi tapi salah (atau member belum pernah bikin PIN sama sekali) → `"invalid pin"`. Verifikasi lewat `auth.VerifyMemberPin()` (fungsi baru di package `auth`, di-export khusus biar dipakai lintas package tanpa expose primitif `hashPin`/`comparePin` yang sengaja tetap private) — cek `mobile_member_pin.pin_hash` milik `member_id` yang sama dengan token, BUKAN dari body.
3. **Saldo cukup** — saldo (`member_balance_ledger.balance_after` terbaru) dibandingkan ke `total_billing`. Kurang → `"insufficient balance"`. Dicek **DUA KALI**: sekali di luar transaksi (early-reject, UX cepat) dan sekali lagi **DI DALAM transaksi** `insertWalletPaidOrder()` (re-check final) — beda dari barrier poin (`min_point_amount`) yang sengaja TIDAK re-check & boleh jadi minus kalau race. Saldo ini duit beneran, race 2 request bersamaan (2 device/klik ganda) yang lolos early-reject yang sama **WAJIB** tetap ketangkep di re-check final → kalau gagal, **SELURUH transaksi di-ROLLBACK** (order batal total, bukan cuma payment-nya gagal).

**Jurnal akuntansi (GL/COA)**: TIDAK ada kode jurnal baru ditulis di sudomobile untuk ini. `member_balance_ledger.transaction_type='payment'` otomatis "dianggap selesai" oleh job `memberbalancejurnal.RunOnce()` (sudocore2) — baris `'payment'` di situ diasumsikan jurnalnya sudah ikut proses endday POS (mirror payment method lain), **asalkan** payment method WALLET_PAYMENT di-setting `coa_accout_id` ke akun yang sama dengan `2.1.07.01 MEMBER WALLET PAYABLE` (setup data master, bukan kode).

**Poin + saldo bisa dipakai bersamaan** — `point_redeem_amount` (lihat section di atas) tetap diproses independen kalau order ini juga pakai promo bersyarat poin.

Response sukses (contoh):

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "order_number": "NOTB2026100114004493",
    "status": "paid",
    "sub_total": "20000.00",
    "total_tax": "0.00",
    "total_discount": "0.00",
    "total_billing": "20000.00",
    "items": [ /* sama struktur kayak CALCULATE.md */ ],
    "payment": {
      "status": "paid",
      "vendor_qr_string": null,
      "vendor_qr_url": null,
      "expired_at": null,
      "failure_reason": null
    }
  }
}
```

**Tervalidasi live (2026-10-01)**: `branch_id=14`/`visit_purpose_id=1`, `payment_method_id=64` (WALLET BALANCE, dev). Guest → `"please login to use wallet payment"`. Token invalid → pesan sama (guest & token invalid tidak dibedakan). Login + saldo cukup → sukses, dicek langsung ke Postgres: `mb_order.status='paid'` + `payment_number` terisi, `mb_order_payment.deduct_member_id` cocok `member_id`, `member_balance_ledger` baris baru `balance_out`/`balance_after` cocok hitungan manual. Login + saldo kurang (order sengaja dibikin gede) → `"insufficient balance"`, dicek tidak ada baris `mb_order` nyangkut sama sekali (rollback bersih). Payment method lain (QRIS) dites bareng, jalur gateway lama tidak ada regresi.

**Tervalidasi live barrier PIN (2026-10-02)**: member id 23 (saldo besar, sudah punya PIN), `branch_id=14`/`visit_purpose_id=1`/`payment_method_id=64`. Tanpa `pin` → `"pin is required for wallet payment"`. `pin` salah (`999999`) → `"invalid pin"`. `pin` benar → sukses (`mb_order.status='paid'`, tersimpan benar di Postgres). Semua data test dibersihkan setelah verifikasi.

Implementasi: `sudomobile/backend/modules/order/order_wallet_payment.go` (`getMemberBalance()`, `insertWalletPaidOrder()`), `sudomobile/backend/modules/order/order_create_handler.go` (percabangan di `Create()`, barrier PIN), `sudomobile/backend/pricing/paymentmethod.go` (`PaymentMethod.IsWalletPayment()`), `sudomobile/backend/modules/auth/generators.go` (`VerifyMemberPin()`, BARU 2026-10-02). Saldo pakai `github.com/shopspring/decimal` (bukan `float64`) — perbandingan nominal uang tidak boleh kena floating-point rounding error.

## Validasi

Semua validasi [`CALCULATE.md`](CALCULATE.md) berlaku (item/package/promo/dll) — DITAMBAH:

- **(2026-09-30, REVISI)** `use_promo_ids` diisi, promo yang di-resolve punya `flag_required_member=true`, TAPI gak login (gak ada token/token invalid) → `"This promo is for members only"`. Promo yang `flag_required_member=false` (default) TETAP bisa dipakai tanpa login.
- `payment_method_id` kosong → `"payment_method_id wajib diisi"`.
- `payment_method_id` gak ketemu / gak lolos filter (gateway-only, scope branch+visit_purpose) → `"payment method tidak ditemukan / tidak berlaku"`.
- **(2026-08-27)** Branch lagi tutup (di luar jam operasional hari ini, `master_branch_ops_setting`) → `"cabang sedang tutup (di luar jam operasional)"`. Dicek pakai `branch.IsOpenNow()` (`modules/branch/branch_handler.go`, di-export biar dipakai lintas modul) — logic-nya SAMA PERSIS yang dipakai buat `flag_status_store_open` di [`GET BRANCH LIST.md`](../MENU/GET%20BRANCH%20LIST.md), cuma sekarang JUGA jadi gerbang keras di sini (sebelumnya cuma info tampilan doang).
- **(2026-08-27)** Branch offline (POS-nya gak/berhenti ngirim heartbeat > 90 detik, lihat `SEND HEARTBEAT.md` di `posv1-laravel`) → `"cabang sedang offline, coba lagi nanti"`. Dicek pakai `heartbeat.IsOnline()` (`backend/heartbeat/heartbeat.go`). Kalau POS-nya offline, gak ada worker yang bakal narik order ini (lihat `PULL MOBILE ORDER.md`) — order ditolak dari awal daripada dibiarin kebuat lalu nyangkut gak pernah diproses.
- Dua validasi di atas **CUMA di `Create()`**, BUKAN di `Calculate()` — preview keranjang tetap bisa dilihat walau branch-nya tutup/offline, gak ada ruginya (gak nyimpen apa-apa).

## Sumber data / implementasi

- `sudomobile/backend/middleware/auth.go` — `OptionalAuth()` (BARU 2026-09-23, lihat "Bug fix" di atas), dipasang di `backend/router.go` ke `orderPublicRouter`.
- `sudomobile/backend/modules/order/order_create_handler.go` — `Create()`, `insertOrder()` (transaksi, sekarang juga insert `order_name` dari `customer_name`, dan `point_redeem_amount` + panggil `redeemMemberPoint()` kalau > 0), `redeemMemberPoint()` (BARU 2026-09-30, insert baris `redeem` ke `member_point_ledger`), `requestPaymentForOrder()`.
- `sudomobile/backend/modules/order/order_payment_status_handler.go` — `refundMemberPoint()` (BARU 2026-09-30, insert baris `redeem_reversal`, dipakai bareng dari `expireOrderAndRefundPoint()` di file ini dan `CancelOrder()` di `order_cancel_handler.go`).
- `sudocore2/migration/231_alter_table_mb_order_add_point_redeem_amount.sql`, `232_alter_table_member_point_ledger_add_redeem_unique.sql`.
- `sudomobile/backend/modules/order/generators.go` — `generateReferenceNumber()` (pola bareng, dipakai `generateOrderNumber()` di sini DAN `generatePaymentNumber()` yang dipakai `finalizeSettledPayment()`, lihat [PAYMENT STATUS.md](PAYMENT%20STATUS.md#format-order_number-dan-payment_number-2026-08-26)), `generateULID()` (pakai `github.com/google/uuid`, BUKAN ULID asli kayak POS punya `Str::ulid()` — cuma butuh unik, sortability-nya emang gak dipakai logic manapun).
- `sudomobile/backend/modules/order/payment_gateway_client.go` — HTTP client ke service `payment`, mirror kontrak `payment/backend/modules/paymentgateway/paymentgateway_dto.go`.
- `sudomobile/backend/pricing/paymentmethod.go` — `ResolvePaymentMethod()`, filter SAMA PERSIS `GET PAYMENT METHOD LIST.md`.
- Env baru `PAYMENT_GATEWAY_ENDPOINT` (`sudomobile/backend/config/payment_gateway.go`, default `http://localhost:98`) — mirror `PAYMENT_GATEWAY_ENDPOINT` POS.

### Bug fix: `tax_rate` NULL ke kolom `NOT NULL` (2026-08-25)

`mb_order_detail`/`mb_order_detail_package.tax_rate` di DB itu `NOT NULL DEFAULT 0` — tapi `pricing.ResolveItemTax()` **sengaja** balikin `nil` buat item/sub-item yang gak kena pajak (nil punya makna sendiri di response API: "emang gak ada pajak", beda dari `"0.00"` yang bisa disalahartikan "kena pajak tapi rate-nya 0%"). Sebelum ada fix ini, order yang isinya item **untaxed** (`use_tax` bukan `"vat"`/`"pb1"`) bakal GAGAL ke-insert total (constraint violation) — ketauan pas item test yang selalu dipakai sepanjang sesi ini (`109`) kebetulan selalu `use_tax='pb1'` (taxed), jadi gak pernah ketes kasus untaxed.

Fix-nya di titik **INSERT doang** (`taxRateOrZero()` di `order_create_handler.go`), BUKAN di `pricing` package — konversi `nil`→`"0.00"` cuma di boundary DB, makna `nil` di response API (`Calculate`/menu-tree) tetap utuh gak berubah.

## Tervalidasi live (2026-08-25)

End-to-end pakai service `payment` beneran (Midtrans sandbox asli, dijalanin lokal port `98`) + `sudomobile` (port sementara) + member session token real:

- Order sukses: `branch_id=51`/`visit_purpose_id=7`, item `109` qty 1, `payment_method_id=1` (QRIS, di-scope sementara ke `visit_purpose_id=7` buat tes) → `order_number` ke-generate, `mb_order`(`status=pending`, `sub_total`/`total_tax`/`total_billing` bener), `mb_order_detail` (1 baris, `menu_id=109`), `mb_order_payment_request` (`status=pending`, `amount` cocok `total_billing`) — semua DICEK LANGSUNG ke Postgres, bukan cuma percaya response. QR asli ke-generate (`vendor_qr_string`/`vendor_qr_url`/`expired_at` dari Midtrans).
- `company_id` di `mb_order` dicek cocok sama `master_branch.company_id` branch `51` (`0`, resolve server-side, bukan dari client).
- Validasi: `payment_method_id` kosong → ditolak; `payment_method_id` gak eligible (`999999`) → ditolak; `menu_id` invalid → ditolak SEBELUM order ke-insert sama sekali (gak ada row nyangkut).
- **Skenario gateway down** — service `payment` dimatiin, order baru di-submit → order TETAP sukses kebuat (`code: 0`, `mb_order.status=pending` beneran ada di DB), `mb_order_payment_request.status=failed` (bukan nyangkut `pending`), response `payment.status=failed` + `failure_reason` jelas (pesan koneksi ditolak).

Semua data test (`mb_order`/`mb_order_detail`/`mb_order_payment_request`/`payment_gateway` di DB service `payment`/`master_payment_method_visit_purposes` scope sementara) dibersihkan total setelah verifikasi.

**Regresi buat bug `tax_rate` (2026-08-25)**: `master_item.use_tax` item `109` (+ sub-item package `97`) di-flip sementara jadi string kosong (untaxed) → `create-order` yang sebelumnya bakal gagal (constraint violation), sekarang **sukses** — response `tax_rate: null` (makna API tetap kejaga), dicek langsung ke Postgres `mb_order_detail`/`mb_order_detail_package.tax_rate` beneran kesimpen `0.00` (bukan `NULL`, sesuai constraint kolom). `use_tax` kedua item di-revert balik ke `pb1` setelah verifikasi.

## Status

Selesai buat alur create + request payment. Buat polling status pembayaran (dan jawaban "orderan yang gak dibayar-bayar jadi apa"), lihat [`PAYMENT STATUS.md`](PAYMENT%20STATUS.md). Yang masih belum ada: endpoint **retry payment request** buat order yang `payment.status=failed`/QR expired.
