# QR Order - Payment Status

**Status: SELESAI & tervalidasi live (2026-09-17).**

```
GET /qr-order/order/:order_number/payment-status?db_code=SUDO&company_code=SUDO&branch_code=SBE&visit_purpose_code=ASD
```

**Publik** — tanpa `Authorization`, tanpa `X-App-Setting`. Dipanggil buat **POLLING** (mis. tiap
beberapa detik) sambil QR ditampilin ke customer — versi QR Order dari
[`../MOBILE/ORDER/PAYMENT STATUS.md`](../../MOBILE/ORDER/PAYMENT%20STATUS.md). **REUSE LANGSUNG**
`SyncPaymentStatus()`, fungsi inti yang SAMA PERSIS dipakai member app (live-check ke service
`payment`, idempotency guard, finalisasi `mb_order_payment` pas settlement, sinkronin
`mb_order.status` pas expired) — gak ada satu baris pun logic pembayaran yang ditulis ulang.
Satu-satunya yang beda dari member app: cara ngecek **boleh diliat oleh siapa**.

## Request

`:order_number` di path + 4 kode identitas di query (wajib, aturan sama
[`GET VISIT PURPOSE DETAIL.md`](./03%20GET%20VISIT%20PURPOSE%20DETAIL.md#request)). Gak ada body.

**"Kepemilikan"** (beda dari member app yang ngecek `member_id` token) — order harus
`order_source = 'qr'` **DAN** `branch_id`-nya cocok sama `branch_id` hasil resolve
`branch_code` di query. Order member app (`order_source = 'mobile'`) ATAU order QR **branch
lain** dua-duanya `"order tidak ditemukan"` (1 pesan, gak dibedain — sama filosofi kepemilikan di
member app).

## Response

```json
{ "code": 0, "message": "success", "data": { "order_number": "NOSBE2026091717042429", "status": "pending" } }
```

`status` — salah satu `pending`/`paid`/`expired`/`cancel`/`failed`, sama persis nilai & semantik
[`../MOBILE/ORDER/PAYMENT STATUS.md`](../../MOBILE/ORDER/PAYMENT%20STATUS.md#response) (gateway
`settlement` di-remap jadi `paid` di response, `mb_order.status` beneran ke-update pas itu
kejadian — bukan cuma di response doang).

## Error

| `message` | Kapan |
| --- | --- |
| 4 kode identitas gak valid | Tabel error di [`GET VISIT PURPOSE DETAIL.md`](./03%20GET%20VISIT%20PURPOSE%20DETAIL.md#error) |
| `order tidak ditemukan` | `order_number` gak ada, ATAU ada tapi `order_source != 'qr'`, ATAU `order_source='qr'` tapi `branch_id`-nya beda dari `branch_code` |
| `gagal ambil data order` | Error DB pas lookup awal |
| `belum pernah ada request pembayaran buat order ini` | `mb_order_payment_request` kosong buat order ini (harusnya gak pernah kejadian buat order yang lolos Create — request payment selalu dicoba di situ) |
| `gagal cek status pembayaran` | Error DB/network pas `SyncPaymentStatus()` |

## Tervalidasi live (2026-09-17)

Order QR asli (branch 51/`SBE`, dibuat lewat [`CREATE ORDER.md`](./06%20CREATE%20ORDER.md)):
- Identitas bener → `status: "pending"`.
- `company_code` yang branch-nya bukan miliknya → ketolak di tahap resolusi identitas
  (`"branch bukan milik company ini"`, sebelum sempat ngecek kepemilikan order).
- `order_number` gak ada → `"order tidak ditemukan"`.
- 1 kode identitas kosong (`db_code`) → `"db_code wajib diisi"`.
- **Order member app** (`order_source='mobile'`, branch **SAMA** — 51) dites lewat endpoint QR
  ini → `"order tidak ditemukan"` — kebukti filter `order_source` jalan, bukan cuma `branch_id`.
- **Settlement beneran** — `payment_gateway.status` (service `payment`) ternyata udah
  `settlement` duluan (dev auto-settle, lihat memori `project_payment_gateway_origin`) pas
  dicek — panggil endpoint ini → `status: "paid"`, `mb_order.status`/`payment_number` ke-update,
  `mb_order_payment` ke-insert (`payment_amount`/`payment_method_id` cocok). Panggil **lagi** →
  tetap `"paid"`, `mb_order_payment` TETAP 1 baris (idempotency guard kebukti jalan, gak dobel
  insert).

Semua data test (order + detail + payment + payment_request) dihapus, `branch_heartbeat`/
`master_branch_ops_setting` branch 51 dibalikin persis ke kondisi semula.
