# Order - Get List Promo

```
GET /api/branch/:branch_id/visit-purpose/:visit_purpose_id/promo
```

**PROTECTED** (wajib `Authorization: Bearer <token>`) — daftar promo yang ELIGIBLE buat 1 kombinasi branch+visit_purpose+member yang lagi login. Wajib login karena salah satu filter eligibility butuh `member_type_id` customer.

Lihat [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md) buat penjelasan lengkap mekanisme promo — dokumen ini fokus ke spek endpoint doang.

## Request

`:branch_id`/`:visit_purpose_id` — sama kayak yang dipakai di [`GET VISIT PURPOSE DETAIL.md`](../MENU/GET%20VISIT%20PURPOSE%20DETAIL.md)/[`GET PAYMENT METHOD LIST.md`](../MENU/GET%20PAYMENT%20METHOD%20LIST.md). Gak ada body.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 22,
      "name": "diskon 20%",
      "code": "123123123",
      "type": "percent",
      "type_rupiah_amount": "0.00",
      "type_percent_rate": "20.00",
      "type_percent_limit_amount": "0.00",
      "type_percent_use_limit": false,
      "promo_for": "category",
      "target_ids": [27, 34, 28, 25, 32, 29, 30, 31],
      "min_buy_amount": "0.00",
      "min_point_amount": "0",
      "apply_limit_per_day": 10,
      "used_today": 0
    }
  ]
}
```

- `target_ids` — isinya `category_id`/`sub_category_id`/`item_id` (tergantung `promo_for` promo itu masing-masing, BISA BEDA per promo dalam 1 response). FE boleh pakai ini buat preview visual (misal badge "dapat promo" di menu), tapi **keputusan final tetap di server** pas `POST /api/order/calculate` — target_ids ini bukan jaminan, cuma info.
- `min_buy_amount`/`min_point_amount`/`apply_limit_per_day`/`used_today` — **info mentah, BUKAN filter**. Promo yang subtotal belanjanya belum cukup, atau limit hariannya udah abis, TETAP MUNCUL di list ini (beda dari `Calculate()` yang bakal nolak beneran pas dipakai) — biar FE bisa nampilin syarat/status ("min. belanja Rp50.000", "min. 100 poin", "1/10 kepake hari ini") tanpa harus nebak sendiri kenapa promo gak bisa dipilih.
- Tipe `freeitem` **gak pernah muncul** di list ini (belum didukung sama sekali, lihat `KETENTUAN PROMO.md`).

`branch_id`/`visit_purpose_id` yang bukan angka → `{ "code": 100, "message": "branch_id tidak valid" }` / `"visit_purpose_id tidak valid"`. Kombinasi yang gak ada promo eligible sama sekali → array kosong `[]`, bukan error. Tanpa `Authorization` → `{ "code": 100, "message": "token tidak ditemukan" }`.

## Sumber data

Filter eligibility (barrier struktural #1-8, lihat `KETENTUAN PROMO.md`) di-generate dari konstanta SQL yang SAMA PERSIS dipakai `pricing.ResolvePromo()` (yang dipanggil `Calculate()`) — `pricing.ListEligiblePromos()` cuma versi "semua yang lolos" tanpa filter `mp.id = ?`. Ini SENGAJA biar list & calculate gak pernah drift beda kondisi (`sudomobile/backend/pricing/promo.go`).

`target_ids` diambil batch sekaligus buat semua promo di list (`pricing.FetchPromoTargetIDs()`, mirror `AttachPromoTargetIds()` POS). `used_today` juga batch (`pricing.FetchPromoUsedTodayBatch()`, mirror `AttachUsedToday()` POS) — dihitung dari `mb_order_detail` doang (scope terpisah dari POS, lihat `KETENTUAN PROMO.md`).

## Tervalidasi live (2026-08-25)

Data real: promo id `22` ("diskon 20%", `flag_all_branches`/`flag_all_visit_purposes`/dll semua `true`) muncul konsisten di branch manapun yang dites (`51`, `999999`) — sesuai `flag_all_*` yang emang gak batasin apa-apa.

Data test temporer (`master_promo`/`master_promo_apply_to`/`master_promo_items`/`master_promo_branches` id `27`, dihapus lagi setelah verifikasi): promo `rupiah` target item `109`, `flag_all_branches=false` + `master_promo_branches` cuma isi `branch_id=999`. Query ke `branch_id=51` → promo `27` **GAK muncul** (benar, branch gak cocok). Query ke `branch_id=999` → promo `27` **muncul** dengan `target_ids: [109]` (cocok `master_promo_items`). Juga dites: tanpa `Authorization` → ditolak; `branch_id` non-angka → ditolak.

## Status

Selesai. Dipasangkan sama [`CALCULATE.md`](CALCULATE.md) — alur normalnya: panggil endpoint ini buat nampilin promo yang bisa dipilih customer, lalu `use_promo_ids` hasil pilihan dikirim ke `POST /api/order/calculate`.
