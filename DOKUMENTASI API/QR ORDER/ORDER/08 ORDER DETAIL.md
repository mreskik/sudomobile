# QR Order - Order Detail

**Status: SELESAI & tervalidasi live (2026-09-17).**

```
GET /qr-order/order/:order_number?db_code=SUDO&company_code=SUDO&branch_code=SBJ&visit_purpose_code=DIN
```

**Publik** — tanpa `Authorization`, tanpa `X-App-Setting`. Versi QR Order dari
[`../MOBILE/ORDER/ORDER DETAIL.md`](../../MOBILE/ORDER/ORDER%20DETAIL.md): struk digital (breakdown
item + package dari snapshot `mb_order_detail`) **plus** status pembayaran yang **live-synced** ke
service `payment` tiap dipanggil (`syncPaymentStatus()`, fungsi yang sama) — termasuk **nampilin
ulang QR** buat order yang masih `pending` (customer nutup browser sebelum scan → buka lagi lewat
endpoint ini, bukan minta QR baru). **Bukan** buat polling rutin — itu udah ada endpoint sendiri
yang lebih ringan, [`PAYMENT STATUS.md`](./07%20PAYMENT%20STATUS.md) (**SELESAI**, keputusan 2026-09-17)
— dokumen ini murni buat struk lengkap.

## Request

`:order_number` di path + 4 kode identitas di query (wajib, aturan sama
[`GET VISIT PURPOSE DETAIL.md`](./03%20GET%20VISIT%20PURPOSE%20DETAIL.md#request)). Gak ada body.

**Kepemilikan** (keputusan 2026-09-17: **cukup `order_number`**, tanpa token/no HP) — yang dicek
cuma: order ada, `order_source = 'qr'`, dan `branch_id`-nya = branch dari `branch_code`
(order QR branch lain / order member app → `order tidak ditemukan`). **Risiko diterima**: siapa pun
yang tau/nebak `order_number` (format `NO`+branch_code+YmdHis+2 digit random) bisa liat struk &
QR bayar order itu. Kalau nanti mau diperketat, opsi paling ringan: `order_token` random yang
dibalikin Create & wajib dikirim di sini.

## Response

Sama persis versi member app — lihat
[`../MOBILE/ORDER/ORDER DETAIL.md`](../../MOBILE/ORDER/ORDER%20DETAIL.md#response) buat contoh &
penjelasan (`items[]` snapshot, `payment.status` live-synced dengan side effect update
`mb_order`/`mb_order_payment_request`, QR fields cuma keisi kalau `pending`, `payment_method_*`
dari attempt terbaru). Ditambah:

```json
{
  "order_source": "qr",
  "order_name": "Budi",
  "customer_phone_number": "081234567890",
  "branch_address": "Jl. Boulevard Jakarta Garden City No. 1"
}
```

**Gak ada `table_number`** — dibuang dari scope Create Order v1 (lihat `CREATE ORDER.md`), jadi
gak ada yang bisa di-echo di sini juga. `member_id` gak ada (selalu `NULL` di order QR). `branch_name` pakai `name_qr_order` fallback
`name` (konsisten sama [`GET VISIT PURPOSE DETAIL.md`](./03%20GET%20VISIT%20PURPOSE%20DETAIL.md)).

## Error

- 4 kode identitas gak valid → tabel di [`GET VISIT PURPOSE DETAIL.md`](./03%20GET%20VISIT%20PURPOSE%20DETAIL.md#error).
- Order gak ada / `order_source` bukan `qr` / bukan branch ini → `"order tidak ditemukan"` (1 pesan, sama
  semantik member app).
- Gagal sync ke service `payment` **bukan** fatal — detail tetep dibalikin, `payment.status`
  fallback ke `mb_order.status`, tanpa QR (sama kayak member app).

## Catatan implementasi

Query item/package/payment (`resolveOrderDetailCore()`) diekstrak dari `GetDetail()` member app
(`order_detail_handler.go`, 2026-09-17) biar dipakai bareng TANPA duplikasi — reuse `SyncPaymentStatus()`
yang sama, gak ada logic pembayaran yang ditulis ulang. Header query & pengecekan kepemilikan
**TETAP terpisah** (bukan nambahin field QR ke `orderDetailHeader`) karena kolom yang dibutuhin
genuinely beda: versi member butuh `member_id` (token-based), versi QR butuh `order_source` (buat
kepemilikan) + `order_name`/`branch_address` (buat struk, gak ada di versi member) — pola sama
kayak `qrOrderOwnerRow` (`PAYMENT STATUS.md`). Handler-nya (`order_qr_detail_handler.go`) hidup di
package `order` yang sama kayak Create/Calculate/PaymentStatus.

`order_name` (BUKAN `customer_name`) — kolomnya di-RENAME migration `211` (2026-09-17, sesudah
endpoint ini pertama kali diimplementasi), disamain sama nama kolom yang udah ada di POS,
`tr_order.order_name`. Lihat "Riwayat perubahan skema" di [`CREATE ORDER.md`](./06%20CREATE%20ORDER.md#riwayat-perubahan-skema-migration-sudocore2).

## Tervalidasi live (2026-09-17)

Branch 51 (`SBE`)/company `SUDO`/visit purpose 7 (`ASD`), order dibikin lewat Create Order
(`NOSBE2026091717323582`, item `109` qty 1) lalu langsung di-detail:
- Sukses → `status: "paid"` (auto-settle dev, konsisten sama gotcha yang udah didokumentasiin di
  `PAYMENT STATUS.md`), breakdown item lengkap (`dpp`/`tax_amount`/`total` per item, `packages: []`),
  `order_source: "qr"`, `order_name`/`customer_phone_number` keisi, `branch_address` keisi dari
  `master_branch.address`, `payment.status: "paid"` sinkron (QR fields `null` karena udah gak
  `pending`), `payment_method_name: "QRIS"` dari attempt terbaru.
- `order_number` gak ada → `"order tidak ditemukan"`.
- Order member app beneran (`order_source = 'mobile'`) diakses lewat endpoint QR → `"order tidak
  ditemukan"` (ownership check nolak, walau order-nya valid).
- `db_code` kosong → `"db_code wajib diisi"`. `company_code` salah → `"company tidak ditemukan"`.
- Semua 4 kode lowercase → tetap sukses, hasil identik.

Data test (order + detail + payment request/payment gateway record) dihapus lagi setelah tes,
`branch_heartbeat`/`master_branch_ops_setting` branch 51 direstore ke kondisi semula
(`closed`, gak ada heartbeat) — diverifikasi ulang lewat query.

**Retes abis rename `customer_name` → `order_name` (migration 211):** order baru dibikin lewat
Create Order pakai body `order_name`, di-detail lewat endpoint ini → response balikin
`"order_name": "Budi Rename Test"` (bukan `customer_name`) dengan benar. Data test dihapus &
branch 51 direstore lagi.
