# Menu - Get Best Seller

3 endpoint beda **scope**, sumber data & aturan yang sama:

```
GET /api/menu/best-seller
GET /api/branch/:branch_id/best-seller
GET /api/branch/:branch_id/visit-purpose/:visit_purpose_id/best-seller
```

**PUBLIK** (gak butuh `Authorization`) — info agregat, gak nempel ke member manapun.

## Sumber data & aturan (sama buat ketiganya)

- **`mb_order`/`mb_order_detail` DOANG** (2026-08-25, konfirmasi eksplisit) — **BUKAN** gabung sama `pos_order_detail`. Konsisten sama keputusan scope promo sebelumnya ([`KETENTUAN PROMO.md`](../ORDER/KETENTUAN%20PROMO.md), "scope pemakaian promo terpisah dari POS") — best seller di sini nunjukin popularitas **DI MOBILE APP**, bukan gabungan semua channel.
- Cuma order **`status = 'paid'`** yang dihitung — order `pending`/`cancel`/`expired` bukan penjualan beneran.
- Rentang waktu: **30 hari terakhir** (trending, bukan all-time) — hardcode, belum ada parameter buat ganti rentang.
- `?limit=` opsional, default `10`, di-cap maksimal `50`.

## Kenapa 3 endpoint (bukan 1 dengan filter opsional)

Makin sempit scope-nya, makin lengkap datanya — **harga cuma bisa dikasih tau kalau `menu_template_id`-nya jelas**, dan itu cuma bisa diresolve dari kombinasi `branch_id`+`visit_purpose_id` (sama persis logic `resolve` yang dipakai [`GET VISIT PURPOSE DETAIL.md`](GET%20VISIT%20PURPOSE%20DETAIL.md)/[`CALCULATE.md`](../ORDER/CALCULATE.md)):

| Endpoint | Scope | Ada harga? |
|---|---|---|
| `GET /menu/best-seller` | Global (semua branch+visit_purpose digabung) | ❌ — 1 item bisa punya harga beda-beda tergantung `menu_template`, gak ada 1 angka yang "bener" |
| `GET /branch/:branch_id/best-seller` | 1 branch | ❌ — 1 branch bisa punya lebih dari 1 `visit_purpose` dengan `menu_template` beda-beda |
| `GET /branch/:branch_id/visit-purpose/:visit_purpose_id/best-seller` | 1 branch + 1 visit_purpose | ✅ — `menu_template_id` deterministik di titik ini |

## Response

**Global / by branch** (tanpa harga):

```json
{
  "code": 0,
  "message": "success",
  "data": [
    { "menu_id": 109, "item_name": "MENU PASTRY", "image_src": "", "icon_src": null, "total_qty": 5, "total_orders": 1 },
    { "menu_id": 103, "item_name": "OKINAWA LATTE", "image_src": "", "icon_src": null, "total_qty": 2, "total_orders": 1 }
  ]
}
```

**By branch+visit_purpose** (DENGAN harga):

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "menu_id": 109, "item_name": "MENU PASTRY", "image_src": "", "icon_src": null,
      "total_qty": 5, "total_orders": 1,
      "price": "123123.00", "tax_type": "pb1", "tax_rate": "10.00",
      "dpp": "111930.00", "net_dpp": "111930.00", "tax_amount": "11193.00", "total": "123123.00"
    },
    {
      "menu_id": 103, "item_name": "OKINAWA LATTE", "image_src": "", "icon_src": null,
      "total_qty": 2, "total_orders": 1,
      "price": null, "tax_type": "pb1", "tax_rate": null, "dpp": "", "net_dpp": "", "tax_amount": "", "total": ""
    }
  ]
}
```

- `total_qty` — total qty terjual (`SUM(mb_order_detail.qty)`) dalam 30 hari terakhir.
- `total_orders` — jumlah order UNIK yang mengandung item ini (`COUNT(DISTINCT order_number)`) — beda dari `total_qty` (1 order bisa beli item yang sama lebih dari 1 qty).
- **`price`/`dpp`/`net_dpp`/`tax_amount`/`total` bisa `null`/kosong** walau item-nya tetap muncul di list — artinya item itu PERNAH laku (ada histori penjualan), tapi **SEKARANG udah gak ada** di `menu_template` branch+visit_purpose ini (mis. dihapus dari menu, atau `qr_order` di-nonaktifin). Item tetap ditampilin (histori penjualannya valid), cuma harganya gak bisa dikasih tau.
- Urutan: `total_qty DESC`.

Gak ada penjualan yang match → `data: []`, bukan error.

## Sumber data (query)

```sql
-- global/branch (tanpa harga)
SELECT mod.menu_id, mi.item_name, mi.image AS image_src, mi.icon_src,
	SUM(mod.qty) AS total_qty, COUNT(DISTINCT mod.order_number) AS total_orders
FROM mb_order_detail mod
JOIN mb_order mo ON mo.order_number = mod.order_number
LEFT JOIN master_item mi ON mi.id = mod.menu_id
WHERE mo.status = 'paid' AND mo.created_at >= now() - interval '30 days'
	[AND mo.branch_id = ?]
GROUP BY mod.menu_id, mi.item_name, mi.image, mi.icon_src
ORDER BY total_qty DESC LIMIT ?
```

Versi branch+visit_purpose nambah `LEFT JOIN master_item_conversion_detail`+`master_pricelist_detail` (filter `menu_template_id` hasil resolve, `qr_order=true`, `is_deleted=false` — pola yang sama kayak menu-tree), lalu tiap baris yang `price` gak `null` di-hitung `dpp`/`tax`/`total`-nya lewat `pricing.CalculateLine()` (SATU sumber kebenaran yang sama dipakai `Calculate()`/menu-tree, gak ditulis ulang).

Implementasi: `sudomobile/backend/modules/bestseller/bestseller_handler.go`.

## Tervalidasi live (2026-08-25)

Data test (`mb_order`/`mb_order_detail` insert langsung, dihapus lagi abis verifikasi): 4 order dibuat — item `109` qty `5` (`paid`, 5 hari lalu), item `103` qty `2` (`paid`, 3 hari lalu), item `109` qty `1` (`pending`, 1 hari lalu — HARUS gak kehitung), item `109` qty `3` (`paid`, tapi **40 hari lalu** — HARUS gak kehitung karena di luar window 30 hari).

- **Global** & **by branch**: keduanya balikin `109` (`total_qty=5`) di atas `103` (`total_qty=2`) — order `pending` dan order `>30 hari` KEDUANYA kefilter dengan benar (kalau kehitung, `109` harusnya `total_qty=9`, bukan `5`).
- **By branch+visit_purpose**: sama urutan, DITAMBAH `109` dapet breakdown harga lengkap (`price=123123.00` dari `menu_template=9`), `103` dapet `price=null` (item itu emang gak ada di `menu_template` tersebut) — item tetap muncul di list, cuma harganya kosong sesuai desain.
- `?limit=1` → cuma 1 item. `branch_id` non-angka → ditolak. `visit_purpose_id` gak ketemu → ditolak. Branch tanpa order sama sekali → `[]`.

Semua data test dibersihkan total setelah verifikasi.

## Status

Selesai.
