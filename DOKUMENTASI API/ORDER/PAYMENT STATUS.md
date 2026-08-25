# Order - Check Payment Status

```
GET /api/order/:order_number/payment-status
```

**PROTECTED** (wajib `Authorization: Bearer <token>`) — dipanggil buat **POLLING** (mis. tiap beberapa detik) sambil QR ditampilin ke customer, abis manggil [`POST /api/order/create-order`](CREATE%20ORDER.md). Mirror PERSIS alur `App\Services\PaymentGatewayServices::CheckStatus()` POS (`KIOSK PAYMENT CHECK STATUS.md`), DITAMBAH pengecekan kepemilikan order (customer-facing, beda dari POS yang internal staff).

## Request

`:order_number` — dari response `create-order`. Gak ada body.

## Response

```json
{ "code": 0, "message": "success", "data": { "order_number": "MB51202608250758354ed2f7", "status": "paid" } }
```

`status` — `pending` / `paid` / `cancel` / `failed` / `expired`. **Beda dari status internal gateway** (`payment_gateway`/`mb_order_payment_request`, yang masih pakai istilah Midtrans `settlement`) — sengaja di-remap `settlement` → `paid` di response ini, biar konsisten sama `mb_order.status` (yang emang udah pakai `paid`, bukan `settlement`). Data yang kesimpen di `payment_gateway` dan `mb_order_payment_request.status` **tetap** `settlement` apa adanya, gak ikut berubah — remap cuma di titik response ini. Sama persis pola Kiosk.

Order gak ketemu / bukan punya member yang login → `{ "code": 100, "message": "order tidak ditemukan" }` (2 kasus ini SENGAJA dikasih pesan yang SAMA — biar gak bocorin ke customer A bahwa order_number tertentu itu beneran ada tapi punya customer B). Belum pernah ada request pembayaran buat order ini → `{ "code": 100, "message": "belum pernah ada request pembayaran buat order ini" }`.

## Alur di dalemnya

1. **Cek kepemilikan** — `mb_order.member_id` harus cocok member yang login. Ini TAMBAHAN dari sudomobile, gak ada di Kiosk POS (POS internal staff, gak ada konsep "order ini punya siapa" dari sisi kasir).
2. **Idempotency guard** — cek `mb_order.status` udah `paid` belum. Kalau udah, langsung balikin `paid` **tanpa** ngecek ulang ke gateway atau insert `mb_order_payment` lagi (polling berkali-kali gak dobel proses).
3. Kalau belum, ambil attempt terbaru dari `mb_order_payment_request` (`ORDER BY created_at DESC LIMIT 1`), live-check `GET {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{order_id}`.
4. Status attempt di-update lokal (`mb_order_payment_request.status`) sesuai hasil live-check — **apa adanya** dari gateway (`settlement`, bukan `paid`).
5. Kalau `settlement` → `finalizeSettledPayment()`: insert `mb_order_payment` (FINAL, 1 transaksi) — `payment_method_id`/`payment_amount` diambil dari `mb_order_payment_request` (snapshot pas request dibuat, **BUKAN** dari client/gateway), `payment_gateway_order_id` diisi `order_id` attempt yang settlement. `mb_order.status` diupdate jadi `paid` di transaksi yang SAMA.
6. Kalau `expired` → `mb_order.status` ikut disinkronin jadi `expired` (guard `WHERE status = 'pending'`, biar gak nabrak state lain kayak yang udah `paid`).
7. `pending`/`cancel`/`failed` → dibalikin apa adanya, gak ada state `mb_order` yang perlu disinkronin.

**Jawaban buat "kalau dibiarin terus, orderan yang gak dibayar jadi apa"**: `mb_order.status` TETAP `pending` selamanya SAMPAI endpoint ini dipanggil minimal 1x setelah QR-nya expired. Gak ada job/cron otomatis yang mantau ini sekarang — sinkronisasi status expired murni terjadi pas polling (langkah 6 di atas), sama persis kelakuan Kiosk POS sebelum dibenerin (order yang QR-nya gak pernah discan nyangkut `pending` selamanya sampai endpoint check-status ini dipanggil).

## Sumber data / implementasi

- `sudomobile/backend/modules/order/order_payment_status_handler.go` — `CheckPaymentStatus()`, `finalizeSettledPayment()`.
- `sudomobile/backend/modules/order/payment_gateway_client.go` — `getPaymentGatewayStatus()` (`GET {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{order_id}`).

## Tervalidasi live (2026-08-25)

End-to-end pakai service `payment` beneran (port 98) + `sudomobile` (port sementara):

- Order dibuat via `create-order` → `payment-status` dipanggil pas masih `pending` → balikin `pending` (benar, belum di-apa-apain).
- Status `payment_gateway.status` di-set `settlement` manual (Postgres, simulasi hasil webhook Midtrans) → `payment-status` dipanggil lagi → balikin `paid`. Dicek langsung ke Postgres: `mb_order.status = 'paid'`, `mb_order_payment` ke-insert 1 baris (`payment_method_id`/`payment_amount`/`payment_gateway_order_id` bener), `mb_order_payment_request.status` tetap `settlement` (gak ikut ke-remap, sesuai desain).
- **Idempotency**: `payment-status` dipanggil LAGI abis `paid` → tetap `paid`, `mb_order_payment` **masih 1 baris** (gak nambah lagi, gak insert dobel).
- **Expired**: order test lain, `payment_gateway.status` di-set `expired` manual → `payment-status` balikin `{"status":"expired"}` **dan** `mb_order.status` ikut jadi `expired` (sebelumnya `pending`).
- **Kepemilikan**: member lain (bukan pemilik order) coba akses → `"order tidak ditemukan"` (bukan bocorin "order ini ada tapi bukan punyamu"). `order_number` yang gak ada juga balikin pesan yang sama.

Semua data test (`mb_order`/`mb_order_detail`/`mb_order_payment_request`/`mb_order_payment`/`payment_gateway`/`master_payment_method_visit_purposes` scope sementara) dibersihkan total setelah verifikasi.

## Status

Selesai. Yang masih belum ada: endpoint **retry payment request** (buat order yang `payment.status=failed` pas `create-order`, atau QR-nya keburu expired sebelum dibayar — customer perlu cara minta QR baru buat order yang SAMA, bukan bikin order baru).
