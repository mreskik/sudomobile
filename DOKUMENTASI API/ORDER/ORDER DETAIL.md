# Order - Order Detail

```
GET /api/order/:order_number
```

**PROTECTED** (wajib `Authorization: Bearer <token>`) — detail LENGKAP 1 order: breakdown item (+package), plus status pembayaran yang SELALU FRESH (bukan data statis).

## Kebutuhan (dikonfirmasi 2026-08-25)

1. **Struk digital** — customer liat rincian per-item, pajak, diskon dari order lampau (bukan cuma ringkasan kayak [`ORDER HISTORY.md`](ORDER%20HISTORY.md)).
2. **Nampilin ULANG QR** buat order yang masih `pending` — `vendor_qr_string`/`vendor_qr_url` CUMA dibalikin sekali di response [`create-order`](CREATE%20ORDER.md), gak pernah disimpen lokal (cuma `expired_at` yang kesimpen di `mb_order_payment_request`). Kalau customer nutup app sebelum sempat scan, QR-nya "ilang" dari sisi `sudomobile` — tapi service `payment` **sendiri** tetap nyimpen `vendor_qr_string`/`url` di tabel `payment_gateway`-nya, jadi tinggal di-**live-fetch ulang** (fungsi `syncPaymentStatus()`, SAMA yang dipakai [`PAYMENT STATUS.md`](PAYMENT%20STATUS.md)) — **BUKAN** minta QR baru (retry, sengaja belum dibangun), cuma nampilin ulang yang lama.

## Request

`:order_number` di URL. Gak ada body.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "order_number": "NOSBE2026082610004571",
    "status": "pending",
    "created_at": "2026-08-25T09:41:54.936117+07:00",
    "branch_id": 51,
    "branch_name": "SUDO BREW - EVENT",
    "visit_purpose_id": 7,
    "visit_purpose_name": "asd",
    "customer_phone_number": null,
    "flag_inclusive_tax": true,
    "sub_total": "125566.36",
    "total_discount": "0.00",
    "total_tax": "12556.64",
    "total_billing": "138123.00",
    "items": [
      {
        "menu_id": 109,
        "item_name": "MENU PASTRY",
        "qty": 1,
        "notes": null,
        "price": "123123.00",
        "tax_type": "pb1",
        "tax_rate": "10.00",
        "dpp": "111930.00",
        "net_dpp": "111930.00",
        "tax_amount": "11193.00",
        "total": "123123.00",
        "promo_id": null,
        "discount_percent": "0.00",
        "discount_amount": "0.00",
        "packages": [
          {
            "menu_package_id": 31,
            "item_id": 97,
            "item_name": "HOT OKINAWA LATTE",
            "qty": 1,
            "price": "15000.00",
            "tax_type": "pb1",
            "tax_rate": "10.00",
            "dpp": "13636.36",
            "net_dpp": "13636.36",
            "tax_amount": "1363.64",
            "total": "15000.00"
          }
        ]
      }
    ],
    "payment": {
      "status": "pending",
      "payment_method_id": 1,
      "payment_method_name": "QRIS",
      "vendor_qr_string": "00020101021226620014COM.GO-JEK.WWW...",
      "vendor_qr_url": "https://merchants-app.sbx.midtrans.com/v4/qris/gopay/.../qr-code",
      "expired_at": "2026-08-25T09:56:55+07:00"
    }
  }
}
```

- `items[]`/`items[].packages[]` — snapshot dari `mb_order_detail`/`mb_order_detail_package` (angka FINAL yang beneran kesimpen pas order dibuat, bukan hasil hitung ulang) — beda dari [`CALCULATE.md`](CALCULATE.md) yang ngitung on-the-fly.
- `payment.status` — **live-synced** tiap kali endpoint ini dipanggil (lewat `syncPaymentStatus()`) — kalau order `pending` dan gateway-nya ternyata udah `settlement`/`expired`, status di response DAN di `mb_order`/`mb_order_payment_request` langsung ikut ke-update (SIDE EFFECT yang disengaja — buka detail order otomatis nge-refresh status, sama kayak manggil [`payment-status`](PAYMENT%20STATUS.md) manual).
- `payment.vendor_qr_string`/`url`/`expired_at` — **cuma keisi kalau `payment.status == "pending"`**. Order yang udah `paid`/`cancel`/`expired`/`failed` semuanya `null` (gak relevan lagi).
- `payment.payment_method_id`/`payment_method_name` — dari attempt TERBARU `mb_order_payment_request` (sama pola kayak [`ORDER HISTORY.md`](ORDER%20HISTORY.md)).
- Gagal sinkronisasi status (network ke service `payment` error, atau belum pernah ada attempt payment sama sekali) **BUKAN dianggap fatal** — detail order tetap dibalikin, `payment.status` fallback ke `mb_order.status` apa adanya, tanpa QR. Beda dari [`PAYMENT STATUS.md`](PAYMENT%20STATUS.md) yang emang tujuan utamanya ngecek status makanya error di situ dianggap gagal.

Order gak ketemu / bukan punya member yang login (pesan disamain) → `{ "code": 100, "message": "order tidak ditemukan" }`.

## Sumber data / implementasi

- `sudomobile/backend/modules/order/order_detail_handler.go` — `GetDetail()`.
- Reuse `syncPaymentStatus()` (diekstrak dari `order_payment_status_handler.go` khusus buat ini — dulu logic-nya nempel di `CheckPaymentStatus()` doang, sekarang dipisah biar dipakai bareng).

## Tervalidasi live (2026-08-25)

End-to-end pakai service `payment` beneran (port 98):

- Order dibuat (item + package) → `GET /order/:order_number` balikin breakdown item+package yang cocok sama yang kesimpen, DAN `payment.vendor_qr_string` **PERSIS SAMA** kayak yang dibalikin `create-order` sebelumnya (dicek string-nya identik) — konfirmasi ini QR yang DITAMPILKAN ULANG, bukan diminta baru.
- `payment_gateway.status` di-set `settlement` manual (Postgres) → `GET /order/:order_number` dipanggil lagi → `payment.status` jadi `paid`, QR fields jadi `null`. Dicek langsung ke Postgres: `mb_order.status` BENERAN ikut ke-`paid`, `mb_order_payment` ke-insert (side-effect sinkronisasi jalan, sama kayak `payment-status`).
- Member lain / `order_number` gak ada → `"order tidak ditemukan"` (sama, gak bocorin kepemilikan).

Semua data test dibersihkan total setelah verifikasi.

## Status

Selesai.
