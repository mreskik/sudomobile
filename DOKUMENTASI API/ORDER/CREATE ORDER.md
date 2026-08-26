# Order - CREATE ORDER

```
POST /api/order/create-order
```

**PROTECTED** (wajib `Authorization: Bearer <token>`) — bikin order beneran (insert `mb_order*`) DAN sekaligus minta QR pembayaran ke service `payment`. 1 call dari sisi client, internal-nya 2 langkah backend (konfirmasi 2026-08-24) — lihat bagian "Alur" di bawah.

Body **SAMA PERSIS** kayak [`CALCULATE.md`](CALCULATE.md) DITAMBAH `payment_method_id`/`customer_phone_number` — logic resolve harga/pajak/promo dipakai ULANG persis (fungsi `calculateOrder()` yang sama), jadi breakdown yang tampil pas preview keranjang GAK PERNAH beda sama yang beneran kesimpen/ke-charge.

## Request

```json
{
  "branch_id": 51,
  "visit_purpose_id": 7,
  "payment_method_id": 1,
  "customer_phone_number": "081234567890",
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

## Validasi

Semua validasi [`CALCULATE.md`](CALCULATE.md) berlaku (item/package/promo/dll) — DITAMBAH:

- `payment_method_id` kosong → `"payment_method_id wajib diisi"`.
- `payment_method_id` gak ketemu / gak lolos filter (gateway-only, scope branch+visit_purpose) → `"payment method tidak ditemukan / tidak berlaku"`.

## Sumber data / implementasi

- `sudomobile/backend/modules/order/order_create_handler.go` — `Create()`, `insertOrder()` (transaksi), `requestPaymentForOrder()`.
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
