# Order - Cancel Order

```
POST /api/order/:order_number/cancel
```

**PROTECTED** (wajib `Authorization: Bearer <token>`) — batalin order **sebelum bayar**. Mirror `App\Services\OrderServices::CancelOrder()` + `PaymentGatewayServices::CancelPendingAttempt()` POS (`KIOSK CANCEL ORDER.md`), DITAMBAH pengecekan kepemilikan order (sama alasan kayak [`PAYMENT STATUS.md`](PAYMENT%20STATUS.md)).

Sistem order di `sudomobile` "sekali jalan" — item/diskon/`payment_method_id` udah final pas [`create-order`](CREATE%20ORDER.md), gak ada hold/edit kayak alur kasir POS. Jadi cancel-nya juga simpel: langsung ubah status, gak ada state `hold` yang perlu ditangani terpisah.

## Request

```json
{ "notes": "customer batal" }
```

`:order_number` di URL. `notes` opsional (disimpen ke `mb_order.cancel_notes`), body boleh kosong.

## Alur (race guard, PENTING)

1. **Cek kepemilikan + status** — order harus punya member yang login, DAN `mb_order.status` harus `pending`. Selain itu ditolak duluan, gak sempat nyoba cancel apa-apa ke gateway.
2. **Cancel attempt payment pending (kalau ada)** — `cancelPendingAttempt()`:
   - Gak ada attempt sama sekali, atau attempt terakhir udah bukan `pending` (udah `settlement`/`expired`/`cancel`/`failed` duluan) → no-op, lanjut ke langkah 3.
   - Attempt masih `pending` → **live-check DULU** `GET {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{order_id}` (bukan blind-cancel) — nutup celah race: sistem ini polling (bukan webhook realtime), jadi ada window kecil dimana customer **beneran scan & bayar** QR-nya PERSIS pas endpoint ini diproses.
     - Live-check balikin **`settlement`** → **JANGAN dicancel** — payment di-finalize (fungsi yang SAMA dipakai [`PAYMENT STATUS.md`](PAYMENT%20STATUS.md): insert `mb_order_payment` + `mb_order.status = 'paid'`). Proses cancel order **DIHENTIKAN** di sini, balikin race response (lihat bawah).
     - Live-check balikin status lain yang UDAH bukan `pending` (`expired`/`cancel`/`failed`) → gak perlu dicancel lagi, sinkronin status lokal aja.
     - Live-check balikin `pending` (atau live-check-nya sendiri gagal, mis. network) → `POST {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{order_id}/cancel` beneran dipanggil, `mb_order_payment_request.status` di-update `cancel`.
3. **Update `mb_order`** — `status = 'cancel'`, `cancel_at`/`cancel_notes` keisi (guard `WHERE status = 'pending'` — kalau ternyata udah keubah gara-gara race di langkah 2, update ini gak ngefek/gak nabrak).

## Response

Sukses:

```json
{ "code": 0, "message": "cancel order berhasil", "data": null }
```

Order gak ketemu / bukan punya member yang login (pesan disamain, gak bocorin kepemilikan):

```json
{ "code": 100, "message": "order tidak ditemukan" }
```

Order udah bukan `pending` (udah `paid`/`cancel`/`expired`):

```json
{ "code": 100, "message": "bukan order pending, gak bisa di-cancel" }
```

**Race — ternyata udah kebayar pas mau di-cancel**:

```json
{ "code": 100, "message": "order ternyata sudah dibayar, tidak jadi di-cancel" }
```

## Sumber data / implementasi

- `sudomobile/backend/modules/order/order_cancel_handler.go` — `CancelOrder()`, `cancelPendingAttempt()`.
- `sudomobile/backend/modules/order/payment_gateway_client.go` — `cancelPaymentGateway()` (`POST {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{order_id}/cancel`).
- Reuse `finalizeSettledPayment()` yang sama dipakai [`PAYMENT STATUS.md`](PAYMENT%20STATUS.md) buat kasus race.

## Tervalidasi live (2026-08-25)

End-to-end pakai service `payment` beneran (port 98):

- **Cancel normal, ada attempt pending**: order dibuat (QR ke-generate) → `cancel` dipanggil (`notes: "customer batal"`) → sukses. Dicek: `mb_order.status = 'cancel'`, `cancel_notes` kesimpen, `mb_order_payment_request.status = 'cancel'`, **DAN dicek langsung `GET /payment-gateway/{order_id}` ke service `payment`** → beneran `status: cancel` di Midtrans (bukan cuma klaim lokal).
- **Cancel 2x**: dipanggil lagi abis itu → `"bukan order pending, gak bisa di-cancel"`, gak dobel proses.
- **Order gak ketemu** / **bukan pemilik**: dua-duanya balikin pesan yang sama, `"order tidak ditemukan"`.
- **Race condition** (paling penting): order baru dibuat (QR aktif) → `payment_gateway.status` di-set `settlement` manual di Postgres (simulasi customer bayar tepat sebelum cancel diproses) → `cancel` dipanggil → balikin `"order ternyata sudah dibayar, tidak jadi di-cancel"`, DAN `mb_order.status` beneran jadi `paid` (bukan `cancel`), `mb_order_payment` ke-insert 1 baris — duitnya gak ke-orphan.

Semua data test dibersihkan total setelah verifikasi (`mb_order`/`mb_order_detail`/`mb_order_payment_request`/`mb_order_payment`/`payment_gateway`/`master_payment_method_visit_purposes` scope sementara).

## Status

Selesai.
