# Order - Calculate

```
POST /api/order/calculate
```

**PUBLIK** (2026-09-22, sebelumnya PROTECTED — `Authorization: Bearer <token>` sekarang OPSIONAL, `X-App-Setting` TETAP wajib) — preview breakdown harga/pajak buat isi keranjang, **SEBELUM** order beneran disubmit. Baca-only, gak insert apa pun ke `mb_order*`. Body **SAMA PERSIS** kayak yang dipakai [`POST /api/order/create-order`](CREATE%20ORDER.md) — sengaja dibikin identik biar frontend bisa reuse payload yang sama antara "hitung dulu di keranjang" dan "submit order beneran".

`use_promo_ids` **TIDAK LAGI wajib login secara blok total** (⚠️ REVISI 2026-09-30, lihat [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md) barrier #6c) — sekarang tergantung `master_promo.flag_required_member` PER PROMO: kalau `false` (default), promo tetap bisa dipakai tanpa login; kalau `true`, baru ditolak `"This promo is for members only"`. `member_id` diambil dari session token kalau ada (opsional), BUKAN dari body — dipakai buat resolve `member_type_id`/`tier_level`/saldo poin (`FetchMemberPromoAttrs()` sekarang AMAN dipanggil dengan `memberID=0`, gak nyoba query yang bakal `ErrNoRows` lagi).

**Bug fix (2026-09-23)**: sama gap-nya kayak [`CREATE ORDER.md`](CREATE%20ORDER.md#bug-fix-2026-09-23) — grup route `/order/*` sempat gak punya middleware auth apa pun sejak digeser publik (2026-09-22), jadi token yang dikirim gak pernah ke-resolve (`member_id` selalu `0`). Fix: `middleware.OptionalAuth` dipasang ke `orderPublicRouter`, sekarang token valid beneran resolve `member_id`. **(⚠️ historis)** — sebelum revisi 2026-09-30, bug ini bikin SEMUA promo ditolak `"promo tidak bisa dipakai tanpa login"` walau client beneran login; sekarang efeknya lebih sempit (cuma promo `flag_required_member=true` yang kena kalau `member_id` gagal ke-resolve).

## Request

```json
{
  "branch_id": 51,
  "visit_purpose_id": 7,
  "use_promo_ids": [23],
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

- `branch_id`/`visit_purpose_id` — dipakai buat resolve `master_branch_visit_purpose` → `menu_template_id` (sumber harga sebenarnya, BUKAN `visit_purpose_id` doang — lihat [`GET VISIT PURPOSE DETAIL.md`](../MENU/GET%20VISIT%20PURPOSE%20DETAIL.md)).
- `items[].menu_id`/`qty`/`notes`/`packages` — **cuma identitas + qty**, TANPA harga/pajak. Server resolve ulang semuanya dari DB (gak percaya angka dari client sama sekali).
- `packages[].package_id` — id grup (`master_item_package_group.id`, dari `package_list` di response `GET VISIT PURPOSE DETAIL.md`). `selections[].menu_package_id` — sub-item yang dipilih, `qty`-nya berapa banyak sub-item itu diambil.
- **Harga sub-item package per menu_template (2026-08-26)**: server resolve harga tiap `menu_package_id` lewat `pricing.FetchPackages()` (fungsi yang SAMA dipakai `GET VISIT PURPOSE DETAIL.md`, lihat dokumen itu buat detail lengkap resolusinya) — jadi harga yang kepake buat `dpp`/`tax_amount`/`total` di response ini udah otomatis sesuai `menu_template_id` visit purpose ini (override kalau ada, fallback ke harga header kalau enggak). Gak ada perbedaan behavior antara `Calculate()` (preview) dan `Create()` (submit beneran) — dua-duanya lewat `calculateOrder()` yang sama.
- `use_promo_ids` — **opsional, SEJAJAR `items` (level order, BUKAN nested di tiap item), maksimal 1 elemen (2026-09-30)**. Client cuma bilang "promo mana yang mau dipakai", server yang nyari sendiri baris item di `items[]` mana yang cocok jadi target promo itu (lewat `promo_for`: category/subcategory/item) — client gak perlu tau logic matching-nya. Lihat [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md) buat detail lengkap.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "visit_purpose_id": 7,
    "menu_template_id": 9,
    "flag_inclusive_tax": true,
    "items": [
      {
        "menu_id": 109,
        "item_name": "MENU PASTRY",
        "pricelist_detail_id": 85,
        "category_id": 34,
        "subcategory_id": 19,
        "qty": 2,
        "notes": "less ice",
        "price": "123123.00",
        "tax_type": "pb1",
        "tax_id": 12,
        "tax_rate": "10.00",
        "dpp": "111930.00",
        "net_dpp": "100737.00",
        "tax_amount": "10073.70",
        "total": "110810.70",
        "promo_id": 23,
        "promo_name": "TEST PROMO CALC",
        "discount_amount": "11193.00",
        "packages": []
      }
    ],
    "sub_total": "223860.00",
    "total_tax": "20147.40",
    "total_discount": "11193.00",
    "total_billing": "221621.40",
    "point_redeem_amount": 0
  }
}
```

(Contoh di atas item qty 2 + promo, TANPA package, biar breakdown promo keliatan jelas. `packages[]` tetap bisa dipakai bareng `promo_id` di request yang sama — promo cuma mempengaruhi baris item utama, sub-item package gak ikut kena diskon, lihat contoh dengan package tanpa promo di [`GET VISIT PURPOSE DETAIL.md`](../MENU/GET%20VISIT%20PURPOSE%20DETAIL.md) atau di bagian "Tervalidasi live" sebelumnya di dokumen ini.)

**Penting soal angka per item**: `dpp`/`net_dpp`/`tax_amount`/`total` itu **PER 1 UNIT** (belum dikali `qty`) — biar frontend gampang nampilin "harga satuan" di keranjang. `sub_total`/`total_tax`/`total_billing` di level ATAS itu udah di-scale `qty` (dan buat sub-item package, di-scale `qty` item induk **DIKALI** `qty` selection-nya — 1 porsi menu qty 2, tiap porsi ambil 1 varian, jadi varian-nya kehitung 2x juga).

`point_redeem_amount` (2026-09-30) — SUM `min_point_amount` dari semua promo di `use_promo_ids` yang punya syarat poin (unik per promo, bukan per baris item). Di `Calculate()` (endpoint ini) field ini **cuma informatif** (preview "bakal kepotong segini") — poinnya **belum** kepotong beneran di sini, baca-only. Poin baru BENERAN dipotong kalau order-nya disubmit lewat `Create()` — lihat [`CREATE ORDER.md`](CREATE%20ORDER.md#potong-poin-buat-promo-bersyarat-poin-min_point_amount-2026-09-30) dan [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md) (barrier #12).

## Formula (DPP-first, replika PERSIS `OrderServices.php::RecalculateOrderTotals()` baris 753-767)

Urutan standar PPN: pajak dilepas dulu dari harga kalau `flag_inclusive_tax=true` (`dpp = price / (1 + tax_rate/100)`, kalau exclusive `dpp = price` apa adanya) → diskon dipotong dari `dpp` (`net_dpp = dpp - discount_amount` — `discount_amount` hasil resolve `promo_id`, lihat [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md); `0` kalau item gak minta promo) → pajak final dihitung ULANG dari `net_dpp` (`tax_amount = net_dpp * tax_rate/100`), BUKAN dari harga awal. `total` per unit = `net_dpp + tax_amount`.

Perhitungan pakai `float64` biasa (BUKAN decimal library) — niru PERSIS cara POS (PHP float) ngitung, termasuk potensi floating-point imprecision-nya, konsisten sama prinsip `ResolveItemTax()` yang niru PERSIS behavior POS apa adanya.

Diimplementasikan di `sudomobile/backend/pricing/pricing.go` (`CalculateLine()`) — fungsi SATU-SATUNYA yang dipakai buat hitung harga/pajak di seluruh `sudomobile`, dipakai ulang juga oleh `GET VISIT PURPOSE DETAIL.md` (versi ringan, cuma `tax_amount` per unit tanpa breakdown `dpp`/`net_dpp` di response) dan dipakai lagi sama [`POST /api/order/create-order`](CREATE%20ORDER.md) — **BUKAN 2 implementasi terpisah**, biar harga preview di keranjang gak pernah beda sama harga final pas checkout.

## Validasi

- `branch_id`/`visit_purpose_id` kosong → `"branch_id dan visit_purpose_id wajib diisi"`.
- `items` kosong → `"items tidak boleh kosong"`.
- `visit_purpose_id` gak ketemu/gak cocok scope (`flag_mobile_customer`/`is_active`) → `"visit purpose tidak ditemukan"`.
- `qty` item ≤ 0 → `"qty item wajib lebih dari 0"`.
- `menu_id` gak ketemu di menu_template hasil resolve branch+visit_purpose (atau `item_status` inactive, atau baris `master_pricelist_detail`-nya `qr_order=false`/`is_deleted`) → `"item tidak ditemukan di menu branch/visit purpose ini"`.
- `package_id` gak ada buat item itu → `"package tidak ditemukan buat item ini"`.
- `menu_package_id` gak ada di grup itu → `"pilihan package tidak ditemukan di grup ini"`.
- `qty` selection ≤ 0 → `"qty pilihan package wajib lebih dari 0"`.
- Total qty selection dalam 1 grup di luar `min_qty`/`max_qty` grup itu → `"jumlah pilihan package di luar batas min/max grup"`.
- **(2026-09-30)** `use_promo_ids` lebih dari 1 elemen → `"cuma boleh pakai maksimal 1 promo per order"` — dicek PALING AWAL, sebelum barrier promo lainnya.
- **(2026-09-30, REVISI)** `use_promo_ids` diisi, promo yang di-resolve punya `flag_required_member=true`, TAPI gak login (gak ada token/token invalid) → `"This promo is for members only"`. Promo yang `flag_required_member=false` (default) TETAP bisa dipakai tanpa login.
- `use_promo_ids` gak ketemu / gak lolos eligibility (channel `mobile_customer`, branch, visit_purpose, member_type, hari, jam, periode, `is_active`) → `"promo {id} tidak ditemukan / tidak berlaku"`.
- `use_promo_ids` gak cocok (`promo_for`) ke item MANA PUN di `items[]` → `"promo {id} tidak berlaku buat item apa pun di cart"`.
- Subtotal belanja (SEBELUM diskon apa pun) belum capai `min_buy_amount` promo → `"belanja belum mencapai minimum buat promo {id}"`.
- Saldo poin member belum capai `min_point_amount` promo → `"poin member gak cukup buat promo {id}"`.
- Promo udah kepake `apply_limit_per_day` kali hari ini (dihitung dari `mb_order_detail`, LIHAT catatan scope di bawah) → `"promo {id} udah mencapai limit pemakaian hari ini"`.
- Tipe promo `freeitem` → ditolak (belum didukung, lihat [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md)).

## Promo

`use_promo_ids` — array, **level ORDER, sejajar `items` (BUKAN nested per-item), maksimal 1 elemen (2026-09-30)**. Client cuma nunjuk "promo mana yang dipakai"; server yang cari sendiri baris item di `items[]` yang cocok jadi target promo itu — client gak perlu ngerti logic matching-nya.

Penjelasan lengkap mekanismenya (barrier/validasi, formula per tipe promo, kenapa beda dari POS, kenapa scope-nya terpisah dari POS) ada di dokumen sendiri: **[`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md)**.

## Tervalidasi live (2026-08-24)

Data real `branch_id=51`/`visit_purpose_id=7` (`menu_template_id=9`), item `109` (harga `123123.00`, `use_tax=pb1`, inclusive) qty 2 + package grup `18` (`min_qty=1`/`max_qty=21`) pilih sub-item `31` ("HOT OKINAWA LATTE", harga `15000.00`) qty 1:

- `dpp` item = `111930.00` (= `123123/1.1`), `tax_amount` = `11193.00`, `total` balik ke `123123.00` (konsisten, inclusive round-trip).
- `dpp` package = `13636.36` (= `15000/1.1`), `tax_amount` = `1363.64`.
- `sub_total` = `251132.72` (`2×111930 + 2×13636.36`), `total_tax` = `25113.28`, `total_billing` = `276246.00` — dicek manual, cocok.

Edge case dites: `menu_id` gak ketemu → error yang sesuai. `qty` selection `25` (di luar `max_qty=21`) → ditolak. `branch_id` kosong → ditolak. `menu_package_id` gak ada di grup → ditolak. Request tanpa `Authorization` → `"token tidak ditemukan"` (**catatan: hasil test ini dari 2026-08-24, pas endpoint masih PROTECTED** — sekarang publik, request tanpa `Authorization` tetap diterima sebagai guest, cuma `use_promo_ids` yang ditolak kalau diisi tanpa login, lihat "Bug fix" di atas).

**Promo** (data test dibuat temporer, dihapus lagi setelah verifikasi — `master_promo`/`master_promo_apply_to`/`master_promo_items`/`master_promo_categories`/`master_pricelist_detail`/`mb_order`/`mb_order_detail`, termasuk revert kolom `master_item.item_category` yang sempat diubah sementara buat simulasi 2 item 1 category):

- **Diskon dasar**: promo `percent` 10% target item 109, channel `mobile_customer`. Item 109 qty 2 + `use_promo_ids: [id]` → `dpp=111930.00`, `discount_amount=11193.00` (10% dari dpp), `net_dpp=100737.00`, `tax_amount=10073.70`, `total=110810.70` per unit — `sub_total=223860.00`, `total_tax=20147.40`, `total_discount=11193.00` (TIDAK di-scale qty, niru quirk yang sama kayak POS), `total_billing=221621.40`. Dicek manual, cocok.
- **Validasi dasar**: `promo_id` gak ketemu → ditolak; `min_buy_amount` di-set tinggi → `"belanja belum mencapai minimum..."`; `apply_limit_per_day=1` + 1 baris `mb_order_detail` dummy hari ini → `"...udah mencapai limit pemakaian hari ini"`.
- **Multi-match (1 promo → 2 baris)**: promo `percent` 5% target `category=34`, cart isi 2 item beda (`109` category 34 asli, `103` category diubah sementara jadi 34 buat simulasi) → KEDUANYA dapet diskon 5% sesuai `dpp` masing-masing (`5596.50` buat item 109, `909.09` buat item 103) di 1 request `use_promo_ids: [24]`. Total di response nge-sum benar dari kedua baris.
- **Konflik (2 promo rebutan 1 baris)**: promo category `24` (match ke 109 & 103) + promo item-specific `25` (target `103` doang) dikirim BARENG di `use_promo_ids: [24, 25]` → ditolak `"promo 24 dan 25 sama-sama cocok ke item OKINAWA LATTE -- pilih salah satu"`, response `code: 100`, gak ada partial-apply. **(⚠️ historis, sebelum barrier maksimal 1 promo/order ada, 2026-09-30)** — request kayak gini sekarang bakal ketolak lebih awal sama `"cuma boleh pakai maksimal 1 promo per order"`, gak akan pernah sampai ke pengecekan konflik ini lagi. Dicatat tetap di sini sebagai jejak histori keputusan, bukan behavior yang masih aktif dites.
- **No-match**: promo item-specific `25` (target 103) dikirim tapi cart cuma isi item 109 (gak match) → ditolak `"promo 25 tidak berlaku buat item apa pun di cart"`.

## Status

Endpoint ini `Calculate` (preview). Buat save order beneran (insert ke `mb_order*` + trigger payment gateway), lihat [`POST /api/order/create-order`](CREATE%20ORDER.md).
