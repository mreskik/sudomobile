# Order - Order History

```
GET /api/order/history
```

**PROTECTED** (wajib `Authorization: Bearer <token>`) — list **header** order milik member yang lagi login. Mirror `KIOSK ORDER HISTORY.md` POS, tapi di-scope ke `member_id` (BUKAN `terminal_id` — Kiosk itu 1 device dipakai gantian banyak kasir/customer, `sudomobile` 1 akun = 1 customer, jadi scope-nya otomatis "punya siapa" bukan "dari device mana").

**Belum termasuk list item per-order** — baru header, sama kayak preseden `KIOSK ORDER HISTORY.md`. Detail per-order (isi item) belum dibangun.

## Request

Query param (semua opsional):

```
GET /api/order/history?date_from=2026-08-01&date_to=2026-08-25
```

- `date_from`/`date_to` — format `Y-m-d`, `date_to` **inclusive** (dibandingin sampai `< date_to + 1 hari`). Filter ke `mb_order.created_at`.
- **BEDA dari Kiosk soal default**: Kiosk default ke HARI INI (staff cuma perlu liat transaksi shift berjalan). Di sini **default TANPA batas tanggal** — customer wajar mau liat SEMUA riwayat order-nya, bukan cuma hari ini. `date_from`/`date_to` murni buat filter tambahan kalau riwayatnya udah panjang.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "order_number": "MB5120260825092937e70fd5",
      "status": "cancel",
      "order_in": "2026-08-25T09:29:37.412205+07:00",
      "total_billing": "123123.00",
      "total_item": 1,
      "branch_id": 51,
      "branch_name": "SUDO BREW - EVENT",
      "visit_purpose_id": 7,
      "visit_purpose_name": "asd",
      "payment_method_id": 1,
      "payment_method_name": "QRIS",
      "payment_expired_at": "2026-08-25T09:44:37+07:00"
    }
  ]
}
```

Gak ada order yang match (termasuk member yang emang belum pernah order) → `data: []`, bukan error.

- `payment_method_id`/`payment_method_name`/`payment_expired_at` diambil dari **ATTEMPT TERAKHIR** `mb_order_payment_request` (BUKAN `mb_order_payment`) — sama alasan kayak Kiosk: biar tetap keisi buat order yang masih `pending` (belum kebayar). Berguna kalau nanti dibikin fitur "lanjutkan bayar" dari list history (retry payment sengaja belum dibangun, lihat [`CANCEL ORDER.md`](CANCEL%20ORDER.md)) — `null` semua kalau order itu belum pernah manggil `create-order`'s payment-request step sama sekali (kasusnya jarang, cuma kalau payment gateway gagal duluan pas order dibuat).
- `total_item` — `SUM(qty)` dari `mb_order_detail`, bukan kolom tersimpan.
- Urutan: `mb_order.created_at DESC` (terbaru duluan).

## Sumber data

```sql
SELECT
	mo.order_number, mo.status, mo.created_at AS order_in, mo.total_billing,
	COALESCE(item_count.total_item, 0) AS total_item,
	mo.branch_id, mb.name AS branch_name,
	mo.visit_purpose_id, mvp.name AS visit_purpose_name,
	latest_pr.payment_method_id, mpm.name AS payment_method_name, latest_pr.expired_at AS payment_expired_at
FROM mb_order mo
LEFT JOIN master_branch mb ON mb.id = mo.branch_id
LEFT JOIN master_visit_purpose mvp ON mvp.id = mo.visit_purpose_id
LEFT JOIN (
	SELECT order_number, SUM(qty) AS total_item FROM mb_order_detail GROUP BY order_number
) item_count ON item_count.order_number = mo.order_number
LEFT JOIN LATERAL (
	SELECT payment_method_id, expired_at FROM mb_order_payment_request
	WHERE order_number = mo.order_number ORDER BY created_at DESC LIMIT 1
) latest_pr ON true
LEFT JOIN master_payment_method mpm ON mpm.id = latest_pr.payment_method_id
WHERE mo.member_id = ?
	[AND mo.created_at >= ?::date]
	[AND mo.created_at < (?::date + interval '1 day')]
ORDER BY mo.created_at DESC
```

`LEFT JOIN LATERAL` dipakai buat ambil attempt payment TERBARU per order (1 baris per `order_number`) — lebih ringkas daripada subquery `MAX(created_at)` + join balik yang dipakai POS, sama-sama ngasih hasil final yang sama.

## Tervalidasi live (2026-08-25)

Data test (2 order dibuat, salah satu di-cancel lewat [`CANCEL ORDER.md`](CANCEL%20ORDER.md), dihapus semua abis verifikasi): `GET /order/history` balikin ke-2 order urut `created_at DESC` (order yang lebih baru/di-cancel duluan muncul), `total_item` cocok (`2` buat order qty 2, `1` buat order qty 1), `branch_name`/`visit_purpose_name`/`payment_method_name` ke-join bener.

- Member LAIN yang gak punya order → `data: []` (scope `member_id` bener, gak bocor ke akun lain).
- `date_from` masa depan → `[]`. `date_to` masa lalu → `[]`. `date_from` hari ini → ke-2 order muncul.

## Status

Selesai buat header. Detail per-order (isi item lengkap per baris history) — lihat [`ORDER DETAIL.md`](ORDER%20DETAIL.md).
