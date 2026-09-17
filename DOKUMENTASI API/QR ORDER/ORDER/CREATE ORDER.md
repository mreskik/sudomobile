# QR Order - Create Order

**Status: SELESAI & tervalidasi live (2026-09-17).**

```
POST /qr-order/create-order?db_code=SUDO&company_code=SUDO&branch_code=SBE&visit_purpose_code=ASD
Content-Type: application/json
```

**Publik** — tanpa `Authorization`, tanpa `X-App-Setting` (route-nya di luar group `/api`, lihat
"Catatan implementasi" di bawah — ini beneran kejadian pas dev, bukan cuma teori). Bikin order
beneran (insert `mb_order*`) **dan** sekaligus minta QR pembayaran ke service `payment` — 1 call
dari client, 2 langkah backend, **REUSE LANGSUNG** alur & fungsi inti
[`../MOBILE/ORDER/CREATE ORDER.md`](../../MOBILE/ORDER/CREATE%20ORDER.md)
(`calculateOrder()`/`generateOrderNumber()`/`requestPaymentForOrder()`/`insertOrderItems()` —
package `order` yang sama, cuma header `mb_order` & sumber identitas yang beda). Yang beda:
identitas request (4 kode di query, bukan token+`X-App-Setting`), identitas customer (**tamu**,
bukan member), dan penanda `order_source = 'qr'`. Tabelnya **`mb_order`/`mb_order_detail`/
`_package`/`mb_order_payment_request` yang sama** — pull ke POS, job `orderexpiry`, sync status
pembayaran, format `order_number`/`payment_number` semuanya langsung kepakai tanpa perubahan.

## Request

4 kode identitas wajib di **query param** — aturan & urutan cek-nya sama persis
[`GET VISIT PURPOSE DETAIL.md`](./GET%20VISIT%20PURPOSE%20DETAIL.md#request).

Body:
```json
{
  "order_name": "Budi",
  "customer_phone_number": "081234567890",
  "payment_method_id": 1,
  "items": [
    {
      "menu_id": 109,
      "qty": 2,
      "notes": "less ice",
      "packages": [
        { "package_id": 18, "selections": [{ "menu_package_id": 31, "qty": 1 }] }
      ]
    }
  ]
}
```

- `order_name` — **wajib** (order QR = tamu, gak ada `member_id`; nama ini yang dipakai
  POS/dapur buat manggil pesanan & nempel di struk). Disimpen ke `mb_order.order_name`
  (migration `210`, DIRENAME dari `customer_name` di migration `211` biar sama persis nama kolom
  `tr_order.order_name` di POS — lihat "Riwayat perubahan skema").
- `customer_phone_number` — opsional, apa adanya (tanpa normalisasi, konvensi sudomobile).
- `payment_method_id` — **wajib**, harus lolos filter [`GET PAYMENT METHOD LIST.md`](./GET%20PAYMENT%20METHOD%20LIST.md)
  (gateway-only, scope branch+visit purpose) — resolve pakai fungsi yang sama
  (`pricing.ResolvePaymentMethod()`).
- `items[]` — sama persis [`CALCULATE.md`](./CALCULATE.md).
- **`use_promo_ids` DITOLAK kalau diisi** (`"promo belum didukung di QR Order"`) — bukan diem-diem
  diabaikan. Promo belum ada di QR Order v1 sama sekali.
- **Gak ada `table_number`** — sempat direncanain di draft awal, dibuang dari v1 (masih belum
  diputusin teks bebas vs FK ke table_section) — lihat "Belum dikerjain" di bawah.

## Response

Sukses (payment gateway berhasil diminta):
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "order_number": "NOSBE2026091716552627",
    "status": "pending",
    "sub_total": "111930.00",
    "total_tax": "11193.00",
    "total_discount": "0.00",
    "total_billing": "123123.00",
    "items": [
      {
        "menu_id": 109,
        "item_name": "MENU PASTRY",
        "pricelist_detail_id": 85,
        "category_id": 34,
        "subcategory_id": 19,
        "qty": 1,
        "notes": "",
        "price": "123123.00",
        "tax_type": "pb1",
        "tax_id": 12,
        "tax_rate": "10.00",
        "dpp": "111930.00",
        "net_dpp": "111930.00",
        "tax_amount": "11193.00",
        "total": "123123.00",
        "packages": [],
        "promo_id": null,
        "promo_name": null,
        "discount_percent": "0.00",
        "discount_amount": "0.00"
      }
    ],
    "payment": {
      "status": "pending",
      "vendor_qr_string": "00020101021226620014COM.GO-JEK.WWW...",
      "vendor_qr_url": "https://merchants-app.sbx.midtrans.com/v4/qris/gopay/.../qr-code",
      "expired_at": "2026-09-17T17:10:26+07:00",
      "failure_reason": null
    },
    "order_source": "qr",
    "order_name": "Budi QR"
  }
}
```

`items[]`/`payment{}` **struktur & sumber datanya identik** versi member app — lihat
[`../MOBILE/ORDER/CREATE ORDER.md`](../../MOBILE/ORDER/CREATE%20ORDER.md#response) (termasuk perilaku
`payment.status: "failed"` + `failure_reason` kalau service `payment` gak bisa dihubungi — **order
tetep kebuat**, sama persis semantik member app). Ditambah di `data`: `"order_source": "qr"`,
`"order_name"`.

`order_number` ini **satu-satunya pegangan** customer buat [`ORDER DETAIL.md`](./ORDER%20DETAIL.md)
(keputusan 2026-09-17: tanpa token tambahan — FE wajib simpen di device, mis. localStorage).

## Yang disimpen ke `mb_order` (beda dari member app)

| Kolom | Member app | QR Order |
|---|---|---|
| `order_source` | `'mobile'` | **`'qr'`** |
| `member_id` | wajib (dari token) | **`NULL`** (kolom nullable, migration `208`) |
| `order_name` | selalu `NULL` | **wajib diisi** (migration `210`, di-rename dari `customer_name` di `211`) |
| `customer_phone_number` | opsional | opsional |
| `order_type` | hardcode `'takeaway'` | diresolve dari `master_visit_purpose.kiosk_mode` (`dinein`/`takeaway`), fallback `'dinein'` kalau `NULL`/belum di-set — QR Order di meja itu dine-in secara alami, beda dari member app yang emang takeaway semua |
| `branch_id`/`visit_purpose_id`/`company_id` | dari body + resolve | dari 4 kode query (`qrorder.Resolve()`, server-side, gak pernah dari body) |
| `status`/pajak/`flag_inclusive_tax`/`pax`/fee | sama | sama (`pax` `NULL`, `order_fee`/`service_charge`/`platform_fee`/`delivery_cost` = `0` — gap yang sama kayak member app) |

`mb_order_detail`/`_package`/`mb_order_payment_request` — **identik**, insert lewat fungsi yang
sama (`insertOrderItems()`), gak ada kolom baru.

## Validasi

Urutan cek (berhenti di kegagalan pertama):
1. 4 kode identitas — tabel error di [`GET VISIT PURPOSE DETAIL.md`](./GET%20VISIT%20PURPOSE%20DETAIL.md#error).
2. `order_name` kosong → `"order_name wajib diisi"`.
3. `items` kosong → `"items tidak boleh kosong"`.
4. `payment_method_id` kosong → `"payment_method_id wajib diisi"`.
5. `use_promo_ids` diisi (gak kosong) → `"promo belum didukung di QR Order"`.
6. Branch lagi tutup (`master_branch_ops_setting`, `branch.IsOpenNow()`) →
   `"cabang sedang tutup (di luar jam operasional)"`.
7. Branch offline (POS gak kirim heartbeat, `heartbeat.IsOnline()`) →
   `"cabang sedang offline, coba lagi nanti"`.
8. Validasi item/package (sama persis [`CALCULATE.md`](./CALCULATE.md), lewat `calculateOrder()`
   yang sama) — `"item tidak ditemukan..."`, `"package tidak ditemukan buat item ini"`, dst.
9. `payment_method_id` gak ketemu/gak lolos filter →
   `"payment method tidak ditemukan / tidak berlaku"`.

Poin 6-7 **cuma di Create**, gak ada di rencana `CALCULATE.md` (preview, gak nyimpen apa-apa) —
sama pola kayak member app.

## Alur (reuse fungsi member app, header beda)

1. `calculateOrder(ctx, db, calcReq, 0)` — `memberID=0` aman karena `UsePromoIDs` udah dipastikan
   kosong di validasi #5, cabang promo (`fetchMemberPromoContext`) gak pernah kesentuh.
2. `generateOrderNumber(branchCode)` (`"NO"+branch_code+YmdHis+2 digit`, fungsi member app apa
   adanya) → `insertQROrder()` (versi QR Order dari `insertOrder()` — header beda, detail item
   lewat `insertOrderItems()` yang **sama fungsinya**, diekstrak dari `insertOrder()` biar dipakai
   bareng tanpa duplikasi) dalam 1 transaksi.
3. `requestPaymentForOrder()` (fungsi member app apa adanya) — insert `mb_order_payment_request`
   (`pending`) → `POST {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/qris` → gagal: request
   di-update `failed`, order **gak** di-rollback.

Setelah `paid`, order ditarik POS lewat mekanisme pull yang udah ada, POS bedain lewat
`order_source` (jalur ini **udah disesuaikan**, lihat "Riwayat perubahan skema" bagian 3 di bawah).
Yang gak dibayar sampai `expired_at` diberesin job `orderexpiry`.

## Catatan implementasi

- **Route WAJIB di luar prefix `/api`** — `/qr-order/create-order`, **BUKAN**
  `/api/qr-order/create-order`. Kepentok beneran pas dev (2026-09-17): `middleware.AppSetting`
  didaftarin lewat `root := app.Group("/api", middleware.AppSetting(...))`
  (`backend/router.go`), dan di Fiber v3 itu nge-`Use()` **prefix path** `/api` di level `app` —
  route apa pun yang diawali `/api` tetep ketangkep middleware itu WALAU didaftarin lewat
  `app.Group()` yang beda variabelnya. Percobaan pertama (`/api/qr-order/...`) balikin
  `"X-App-Setting wajib diisi"` ke SEMUA request QR Order, ketuker sama sekali. Pindah prefix ke
  `/qr-order` (di luar `/api`) yang beneran lepas.
- Handler-nya (`QRHandler`/`qrHandler`, `order_qr_create_handler.go`) **hidup di package `order`
  yang sama** kayak member app (bukan package `qrorder` terpisah) — biar bisa reuse langsung
  `calculateOrder`/`generateOrderNumber`/`requestPaymentForOrder`/`insertOrderItems`/
  `taxRateOrZero`/`nullIfEmpty` (semua unexported) tanpa export apa pun / tanpa duplikasi. Yang
  di package terpisah (`backend/modules/qrorder/`) cuma resolusi 4 kode identitas
  (`qrorder.Resolve()`) — itu yang genuinely dipakai bareng SEMUA endpoint QR Order nantinya,
  bukan cuma Create.
- `order_type` diresolve lewat helper baru `resolveQROrderType()` — query `kiosk_mode` sendiri
  (bukan bagian `qrorder.Resolve()`, karena cuma dipakai Create, bukan endpoint lain).

## Belum dikerjain (di luar scope Create Order v1)

- **`table_number`** — masih belum diputusin (teks bebas vs FK ke `table_section`), dibuang dari
  request/response Create sampai jelas. Kalau nanti masuk: kolom baru `mb_order.table_number` +
  field baru di body.
- **Promo** — lihat "Belum dikerjain" di [`KETENTUAN QR ORDER.md`](../KETENTUAN%20QR%20ORDER.md).

**Cek ikutan job tier/point — SUDAH DICEK (2026-09-17), AMAN.** Ditelusuri jalur lengkapnya:
`mb_order.member_id NULL` (order QR) → ditarik `APIANDORDER` `GetPending()`
(`mobileorder_service.go`, `MemberID *int64`, gak difilter) → POS `MobileOrderPullServices.php:169`
(`'member_id' => $order['member_id'] ?? null`, NULL kesimpen apa adanya ke `tr_order` lokal) →
`PushDataServices::pushDataOrder()` → APIANDORDER `pushdata_service.go` `PushPOSOrder()` → upsert ke
`pos_order` (`MemberID *int32`, NULL tetep kebawa). Jadi baris `pos_order.member_id IS NULL` dari
order tamu QR **beneran ada** di data. Tapi SEMUA konsumennya udah aman:
- `pointcheck` (`sudocore2/backend/modules/pointcheck/point_check_service.go:160`) —
  `WHERE status = 'paid' AND member_id IS NOT NULL AND point_checked_at IS NULL` — guard eksplisit.
- `membertierevaluation` (`sudocore2/backend/modules/membertierevaluation/member_tier_evaluation_service.go:250,255`)
  — `pos_order`/`barber_booking` dua-duanya `WHERE ... member_id IS NOT NULL` sebelum `GROUP BY`.
- `sudobarber` `TierSpending` (`backend/modules/member/member_tier.go:113,118`) & sudomobile
  `TierSpending` (`backend/modules/account/tier_spending_handler.go:93,98`) — `member_id = ?`
  ke-bind ke 1 member yang lagi login, gak mungkin match baris `NULL` walau tanpa `IS NOT NULL`
  eksplisit.

Gak ada job/query lain yang megang `pos_order`/`mb_order` buat itung poin/tier (`main.go` sudocore2
cuma daftarin 3 job: `pointcheck`/`memberbalancejurnal`/`membertierevaluation`,
`memberbalancejurnal` gak pernah baca `pos_order` sama sekali). Risiko ini ditutup, bukan cuma
"belum ketemu bukti" — udah dibaca kodenya, emang amannya bukan kebetulan.

## Riwayat perubahan skema (migration sudocore2)

Urutan implementasi, semua **SELESAI & di-apply live + dicopy ke `ROAD TO UPLOAD`**:

**208 — `mb_order.member_id` nullable.** 3 konsumen Go yang tadinya ERROR kalau ketemu `NULL`
(`int64` non-pointer gagal `Scan()`) disesuaikan bareng: `APIANDORDER` `PendingOrder.MemberID`
(**paling kritis** — `GetPending()` narik banyak order dalam 1 `Scan()`, 1 baris `NULL` bisa
gagalin SELURUH batch pull branch itu), sudomobile `orderOwnerRow` (`order_payment_status_handler.go`,
dipakai bareng `order_cancel_handler.go`) + `orderDetailHeader` (`order_detail_handler.go`) — semua
jadi `*int64`. `order_history_handler.go` (filter SQL `WHERE member_id = ?`), `tr_order.member_id`
POS, dan `MobileOrderPullServices::processOrder()` POS **gak perlu disentuh** (udah aman dari
sononya).

**209 — `mb_order.order_source VARCHAR(20) NOT NULL DEFAULT 'mobile'`.** Namanya `order_source`
(bukan `order_from` seperti draft awal) — disamain sama kolom yang **udah ada** di POS,
`tr_order.order_source` (`posv1-laravel` migration `2026_04_23_100709_transaction_data.php:29`),
yang komentarnya sendiri udah nyiapin `// pos, qr, kiosk` dari awal. Sengaja plain `VARCHAR`
tanpa `CHECK` constraint (mirror `tr_order.order_source`). Isinya literal di backend tiap endpoint
Create (`'mobile'` member app, `'qr'` QR Order) — bukan field dari client. 34 baris existing
ke-backfill `'mobile'`.

**Pull ke POS disesuaikan bareng 209** — `APIANDORDER` `GetPending()`/`PendingOrder` nambahin
`order_source` ke payload; POS `MobileOrderPullServices::processOrder()`
(`app/Services/MobileOrderPullServices.php:156`) baca dari payload
(`$order['order_source'] ?? 'mobile'`), gak hardcode lagi.

**210 — `mb_order.customer_name VARCHAR(255)`, nullable.** Member app gak pernah ngisi (identitas
dari `member_id`), QR Order wajib isi (divalidasi di aplikasi, bukan `NOT NULL` di DB — pola sama
kayak `customer_phone_number`).

**211 — `mb_order.customer_name` di-RENAME jadi `order_name`.** Disamain sama nama kolom yang
**udah ada** di POS, `tr_order.order_name` (persis sebelahan sama `tr_order.order_source` yang
udah lebih dulu disamain namanya di `209`) — biar pas jalur pull `mb_order` → `tr_order` digarap,
nama field-nya udah cocok di kedua sisi tanpa mapping/alias. Cuma rename kolom (nullable & data gak
berubah); konsumen Go (`qrCreateOrderRequest.OrderName`/`qrCreateOrderResult.OrderName`/
`qrOrderDetailHeader.OrderName`/`qrOrderDetailResult.OrderName`, JSON `order_name`) diubah bareng.

**Pull ke POS disesuaikan bareng 211 — SELESAI & tervalidasi live (2026-09-17).** Sebelum ini,
`MobileOrderPullServices.php` ngisi `tr_order.order_name` cuma dari `member_name` (JOIN live ke
`master_member` di `APIANDORDER` `GetPending()`) — order tamu QR bakal masuk POS dengan nama
kosong, karena `member_name`-nya emang gak ada. Diperbaiki: `APIANDORDER` `GetPending()`/
`PendingOrder` nambahin `mo.order_name` ke payload (`OrderName *string`, NULL apa adanya — BUKAN
di-`COALESCE` kayak `member_name`, biar fallback `??` di PHP jalan bener), POS
`MobileOrderPullServices::processOrder()` (`app/Services/MobileOrderPullServices.php:154`) jadi
`$order['order_name'] ?? $order['member_name'] ?? ''` — 2 sumber ini gak pernah keisi bareng (order
QR selalu isi `order_name`/gak pernah punya member; order member app selalu `order_name` NULL/isi
`member_name` dari JOIN), jadi urutan fallback aman buat kedua channel.

Tervalidasi live lewat `get_pending` (bukan cuma `php -l`, beneran manggil endpoint-nya): order QR
baru (`order_source='qr'`) → `"order_name":"Budi Pull Test","member_name":""`. Order member app
lama yang beneran pernah ke-pull (`order_source='mobile'`, `pulled_at` sempet di-NULL-in sementara
buat munculin lagi di `get_pending`, dibalikin abis tes) → `"order_name":null,"member_name":"Reski
Kurniawan"` — fallback ke `member_name` kebukti masih jalan normal, gak keganggu perubahan ini.
Sisi POS (`MobileOrderPullServices.php`) sendiri masih sama kayak fix `order_source` — dicek syntax
(`php -l`) doang, belum lewat `artisan mobile-order:pull` beneran (butuh dayshift/terminal/printer
aktif, di luar lingkup sesi ini).

## Risiko yang diterima

- **Publik tanpa hambatan** — siapa pun bisa nembak endpoint ini bikin order kosong-harga/spam
  (order `pending` yang gak dibayar bakal `expired` sendiri lewat `orderexpiry`, tapi tetep
  numpuk). Sama kelas risikonya kayak Booking Create customer di sudobarber — barier (captcha/rate
  limit) ditunda, dicatat sebagai pending.
- **`order_number` doang buat akses detail** (keputusan 2026-09-17) — lihat [`ORDER DETAIL.md`](./ORDER%20DETAIL.md).

## Tervalidasi live (2026-09-17)

Branch 51 (`SBE`)/company `SUDO`/visit purpose 7 (`ASD`, `kiosk_mode` `NULL`), item `109`,
`payment_method_id=1` (QRIS) — jam operasional & heartbeat branch 51 dipinjam sementara buat tes
(branch ini gak ada di keduanya dari awal), dibalikin persis abis selesai:

- **Sukses penuh, end-to-end pakai service `payment` beneran** (Midtrans sandbox) — `order_number`
  ke-generate, QR asli ke-generate (`vendor_qr_string`/`vendor_qr_url`/`expired_at`). Dicek
  langsung ke Postgres: `mb_order` (`member_id` `NULL`, `order_name`/`customer_phone_number`
  kesimpen, `order_source='qr'`, `order_type='dinein'` — kebukti bener resolve dari `kiosk_mode`
  `NULL` → fallback), `mb_order_detail` (1 baris, `menu_id=109`), `mb_order_payment_request`
  (`status=pending`, `amount` cocok `total_billing`).
- 4 kode identitas: masing-masing kosong → 4 pesan `"... wajib diisi"` yang beda-beda bener.
  `company_code` salah → `"company tidak ditemukan"`. `branch_code` salah →
  `"branch tidak ditemukan"`. `branch_code` valid tapi `company_code` company lain →
  `"branch bukan milik company ini"`. `visit_purpose_code` salah →
  `"visit purpose tidak ditemukan"`. Kombinasi 4 kode **lowercase semua** → tetep sukses
  (case-insensitive kebukti jalan).
- `order_name` kosong → `"order_name wajib diisi"`. `items: []` →
  `"items tidak boleh kosong"`. `payment_method_id` kosong → `"payment_method_id wajib diisi"`.
  `use_promo_ids: [23]` diisi → `"promo belum didukung di QR Order"` (ditolak, bukan diabaikan).

Semua data test (2 order + detail + payment_request) dihapus lagi, `branch_heartbeat`/
`master_branch_ops_setting` branch 51 dibalikin persis ke kondisi semula. `go build`/`go vet`
bersih.

**Retes abis rename `customer_name` → `order_name` (migration 211):** body `{"order_name": "Budi
Rename Test", ...}` → sukses, response balikin `"order_name": "Budi Rename Test"`. Body pakai field
lama `customer_name` (bukan `order_name`) → `"order_name wajib diisi"` (field lama gak lagi
dikenali, sesuai ekspektasi — bukan diterima diam-diam sebagai alias). Data test dihapus lagi,
`branch_heartbeat`/`master_branch_ops_setting` branch 51 direstore lagi.

**Belum dites**: `cabang tutup`/`cabang offline` di endpoint QR Order ini spesifik (dua-duanya
REUSE fungsi member app apa adanya — `branch.IsOpenNow()`/`heartbeat.IsOnline()` — udah pernah
tervalidasi di konteks member app, dan sempet ketemu juga secara gak sengaja di awal sesi test ini
sebelum jam operasional branch 51 dipinjam). `artisan mobile-order:pull` POS end-to-end (butuh
dayshift/terminal worker/printer aktif, di luar lingkup perubahan sesi ini).
