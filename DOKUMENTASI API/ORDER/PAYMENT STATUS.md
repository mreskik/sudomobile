# Order - Check Payment Status

```
GET /api/order/:order_number/payment-status
```

**PROTECTED** (wajib `Authorization: Bearer <token>`) — dipanggil buat **POLLING** (mis. tiap beberapa detik) sambil QR ditampilin ke customer, abis manggil [`POST /api/order/create-order`](CREATE%20ORDER.md). Mirror PERSIS alur `App\Services\PaymentGatewayServices::CheckStatus()` POS (`KIOSK PAYMENT CHECK STATUS.md`), DITAMBAH pengecekan kepemilikan order (customer-facing, beda dari POS yang internal staff).

## Request

`:order_number` — dari response `create-order`. Gak ada body.

## Response

```json
{ "code": 0, "message": "success", "data": { "order_number": "NOSBE2026082610004571", "status": "paid" } }
```

`status` — `pending` / `paid` / `cancel` / `failed` / `expired`. **Beda dari status internal gateway** (`payment_gateway`/`mb_order_payment_request`, yang masih pakai istilah Midtrans `settlement`) — sengaja di-remap `settlement` → `paid` di response ini, biar konsisten sama `mb_order.status` (yang emang udah pakai `paid`, bukan `settlement`). Data yang kesimpen di `payment_gateway` dan `mb_order_payment_request.status` **tetap** `settlement` apa adanya, gak ikut berubah — remap cuma di titik response ini. Sama persis pola Kiosk.

Order gak ketemu / bukan punya member yang login → `{ "code": 100, "message": "order tidak ditemukan" }` (2 kasus ini SENGAJA dikasih pesan yang SAMA — biar gak bocorin ke customer A bahwa order_number tertentu itu beneran ada tapi punya customer B). Belum pernah ada request pembayaran buat order ini → `{ "code": 100, "message": "belum pernah ada request pembayaran buat order ini" }`.

## Alur di dalemnya

1. **Cek kepemilikan** — `mb_order.member_id` harus cocok member yang login. Ini TAMBAHAN dari sudomobile, gak ada di Kiosk POS (POS internal staff, gak ada konsep "order ini punya siapa" dari sisi kasir).
2. **Idempotency guard** — cek `mb_order.status` udah `paid` belum. Kalau udah, langsung balikin `paid` **tanpa** ngecek ulang ke gateway atau insert `mb_order_payment` lagi (polling berkali-kali gak dobel proses).
3. Kalau belum, ambil attempt terbaru dari `mb_order_payment_request` (`ORDER BY created_at DESC LIMIT 1`), live-check `GET {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{order_id}`.
4. Status attempt di-update lokal (`mb_order_payment_request.status`) sesuai hasil live-check — **apa adanya** dari gateway (`settlement`, bukan `paid`).
5. Kalau `settlement` → `finalizeSettledPayment()`: generate `payment_number` (baru, lihat [format-nya di bawah](#format-order_number-dan-payment_number-2026-08-26)), **update `mb_order`** (`status = 'paid'`, `payment_number` diisi) DULUAN, baru **insert `mb_order_payment`** (FINAL, `payment_method_id`/`payment_amount` diambil dari `mb_order_payment_request` — snapshot pas request dibuat, **BUKAN** dari client/gateway; `payment_gateway_order_id` diisi `order_id` attempt yang settlement) — urutan ini WAJIB (`mb_order_payment.payment_number` FK ke `mb_order.payment_number`, kebalik kena FK violation). Semua dalam 1 transaksi.
6. Kalau `expired` → `mb_order.status` ikut disinkronin jadi `expired` (guard `WHERE status = 'pending'`, biar gak nabrak state lain kayak yang udah `paid`).
7. `pending`/`cancel`/`failed` → dibalikin apa adanya, gak ada state `mb_order` yang perlu disinkronin.

**Jawaban buat "kalau dibiarin terus, orderan yang gak dibayar jadi apa"**: `mb_order.status` TETAP `pending` selamanya SAMPAI endpoint ini dipanggil minimal 1x setelah QR-nya expired. Gak ada job/cron otomatis yang mantau ini sekarang — sinkronisasi status expired murni terjadi pas polling (langkah 6 di atas), sama persis kelakuan Kiosk POS sebelum dibenerin (order yang QR-nya gak pernah discan nyangkut `pending` selamanya sampai endpoint check-status ini dipanggil).

## Format `order_number` dan `payment_number` (2026-08-26)

Disepakati bareng, migration `sudocore2/cmd/migration/120_alter_table_mb_order_add_payment_number.sql`:

| | `order_number` | `payment_number` |
|---|---|---|
| Format | `"NO"` + branch_code + timestamp(`YmdHis`) + 2 digit random | `"QR"` + branch_code + timestamp(`YmdHis`) + 2 digit random |
| Kapan digenerate | **Pas order dibuat** (`create-order`) | **Pas payment settlement** (`finalizeSettledPayment()`) — order yang gak pernah kebayar (expired/cancel) gak akan pernah punya `payment_number`, kolomnya tetap `NULL` |
| branch_code | resolve dari `branch_id` (`master_branch.code`) | resolve dari `branch_id` (`master_branch.code`), lewat JOIN `mb_order` di `finalizeSettledPayment()` |
| Fungsi generator | `generateOrderNumber()` | `generatePaymentNumber()` |

Keduanya niru pola PENAMAAN yang dipakai POS (`OrderServices::GenerateOrderNumber()` prefix `"NO"`, `PaymentServices::GenerateOrderNumber()` prefix `"PS"` — di sini dipilih `"QR"` karena mobile cuma dukung payment gateway QRIS, lihat [`GET PAYMENT METHOD LIST.md`](../MENU/GET%20PAYMENT%20METHOD%20LIST.md)), TAPI beda titik pemicu: POS generate `payment_number` dari 1 aksi eksplisit (kasir klik "bayar", `SavePayment()`), `sudomobile` generate dari deteksi PASIF (salah satu dari 3 entry point yang funnel ke `finalizeSettledPayment()` — endpoint ini, `ORDER DETAIL.md`, atau job `orderexpiry`) — aman karena tetap 1 titik kode, ke-generate sekali doang per order (row `mb_order_payment` juga cuma sekali).

`mb_order_payment` (dulu FK ke `mb_order.order_number`) sekarang FK ke `mb_order.payment_number` — niru PERSIS pola POS (`tr_order_payment` link ke `tr_order` lewat `payment_number`, bukan `order_number` — `tr_order_payment` malah gak nyimpen `order_number` sama sekali).

**⚠️ Random 2 digit itu SEMENTARA** — jauh lebih kecil entropinya dari skema lama `order_number` (6 hex, ±16 juta kemungkinan), yang sengaja dipilih waktu itu buat ngatasin resiko tabrakan concurrency (banyak device customer beda bisa nembak/settlement bareng di detik yang sama, buat branch yang sama). 2 digit (100 kemungkinan) ngebalikin resiko itu, DITANDAI eksplisit sebagai open item, bukan dianggap aman permanen — bakal direvisi belakangan (`order_number` dan `payment_number` sekaligus, bareng).

## Sumber data / implementasi

- `sudomobile/backend/modules/order/order_payment_status_handler.go` — `CheckPaymentStatus()`, `finalizeSettledPayment()`.
- `sudomobile/backend/modules/order/generators.go` — `generateReferenceNumber()`, `generateOrderNumber()`, `generatePaymentNumber()`.
- `sudomobile/backend/modules/order/payment_gateway_client.go` — `getPaymentGatewayStatus()` (`GET {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{order_id}`).

## Tervalidasi live (2026-08-25)

End-to-end pakai service `payment` beneran (port 98) + `sudomobile` (port sementara):

- Order dibuat via `create-order` → `payment-status` dipanggil pas masih `pending` → balikin `pending` (benar, belum di-apa-apain).
- Status `payment_gateway.status` di-set `settlement` manual (Postgres, simulasi hasil webhook Midtrans) → `payment-status` dipanggil lagi → balikin `paid`. Dicek langsung ke Postgres: `mb_order.status = 'paid'`, `mb_order_payment` ke-insert 1 baris (`payment_method_id`/`payment_amount`/`payment_gateway_order_id` bener), `mb_order_payment_request.status` tetap `settlement` (gak ikut ke-remap, sesuai desain).
- **Idempotency**: `payment-status` dipanggil LAGI abis `paid` → tetap `paid`, `mb_order_payment` **masih 1 baris** (gak nambah lagi, gak insert dobel).
- **Expired**: order test lain, `payment_gateway.status` di-set `expired` manual → `payment-status` balikin `{"status":"expired"}` **dan** `mb_order.status` ikut jadi `expired` (sebelumnya `pending`).
- **Kepemilikan**: member lain (bukan pemilik order) coba akses → `"order tidak ditemukan"` (bukan bocorin "order ini ada tapi bukan punyamu"). `order_number` yang gak ada juga balikin pesan yang sama.

Semua data test (`mb_order`/`mb_order_detail`/`mb_order_payment_request`/`mb_order_payment`/`payment_gateway`/`master_payment_method_visit_purposes` scope sementara) dibersihkan total setelah verifikasi.

**Tervalidasi buat perubahan `payment_number` (2026-08-26)**: dites lewat `go test` sementara yang manggil `finalizeSettledPayment()` LANGSUNG (kode produksi beneran, bukan reimplementasi SQL manual) — `branch_id=51` → `branch_code` ke-resolve otomatis jadi `"SBE"` (dari `master_branch.code`), `payment_number` ke-generate `QRSBE...` (prefix bener), `mb_order.status` jadi `paid` + `payment_number` kesimpen, `mb_order_payment` ke-insert dengan FK yang nyambung ke `mb_order.payment_number` (query balik ketemu row-nya). File test dihapus lagi abis verifikasi (`verify_manual_test.go`, gak pernah masuk git).

## Status

Selesai. Yang masih belum ada: endpoint **retry payment request** (buat order yang `payment.status=failed` pas `create-order`, atau QR-nya keburu expired sebelum dibayar — customer perlu cara minta QR baru buat order yang SAMA, bukan bikin order baru).
