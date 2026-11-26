# Order - Calculate

```
POST /api/order/calculate
```

**PUBLIK** (2026-09-22, sebelumnya PROTECTED — `Authorization: Bearer <token>` sekarang OPSIONAL, `X-App-Setting` TETAP wajib) — preview breakdown harga/pajak buat isi keranjang, **SEBELUM** order beneran disubmit. Baca-only, gak insert apa pun ke `mb_order*`. Body **SAMA PERSIS** kayak yang dipakai [`POST /api/order/create-order`](CREATE%20ORDER.md) — sengaja dibikin identik biar frontend bisa reuse payload yang sama antara "hitung dulu di keranjang" dan "submit order beneran".

`use_promo_ids` **TIDAK LAGI wajib login secara blok total** (⚠️ REVISI 2026-09-30, lihat [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md) barrier #6c) — sekarang tergantung `master_promo.flag_required_member` PER PROMO: kalau `false` (default), promo tetap bisa dipakai tanpa login; kalau `true`, baru ditolak `"This promo is for members only"`. `member_id` diambil dari session token kalau ada (opsional), BUKAN dari body — dipakai buat resolve `member_type_id`/`tier_level`/saldo poin (`FetchMemberPromoAttrs()` sekarang AMAN dipanggil dengan `memberID=0`, gak nyoba query yang bakal `ErrNoRows` lagi).

**Bug fix (2026-09-23)**: sama gap-nya kayak [`CREATE ORDER.md`](CREATE%20ORDER.md#bug-fix-2026-09-23) — grup route `/order/*` sempat gak punya middleware auth apa pun sejak digeser publik (2026-09-22), jadi token yang dikirim gak pernah ke-resolve (`member_id` selalu `0`). Fix: `middleware.OptionalAuth` dipasang ke `orderPublicRouter`, sekarang token valid beneran resolve `member_id`. **(⚠️ historis)** — sebelum revisi 2026-09-30, bug ini bikin SEMUA promo ditolak `"promo tidak bisa dipakai tanpa login"` walau client beneran login; sekarang efeknya lebih sempit (cuma promo `flag_required_member=true` yang kena kalau `member_id` gagal ke-resolve).

**⚠️ BREAKING CHANGE (2026-11-19)** — gap Service Charge & Order Fee yang sebelumnya SELALU `0` sekarang beneran dihitung, DAN field harga per item di-rename:
- `dpp`/`net_dpp` (lama) → **`net_price`/`dpp`** (baru). Makna TIDAK berubah, cuma nama kolom yang digeser: `net_price` = harga sebelum diskon (basis diskon persen), `dpp` = basis pajak (setelah diskon). Lihat bagian Formula di bawah buat detail lengkap kenapa.
- Field baru per item: `service_charge_id` (FK `master_tax`, dari `master_branch_visit_purpose.service_charge` — level VISIT PURPOSE, SAMA buat semua item dalam 1 order, BUKAN per-item custom kayak POS), `service_charge_rate`, `service_charge_amount`.
- `total` per item sekarang `dpp + tax_amount + service_charge_amount` (dulu cuma `dpp + tax_amount`, sebelum rename `net_dpp + tax_amount`).
- Field baru level order: `total_service_charge` (SUM `service_charge_amount` semua baris, sudah di-scale `qty`), `order_fee` (nominal rupiah TETAP dari `master_branch_visit_purpose.order_fee`, BUKAN rate/persen, ditambahkan apa adanya ke `total_billing` — TIDAK di-scale apa pun).
- `total_billing` sekarang ikut termasuk `total_service_charge` (lewat `total` per item) dan `order_fee` (ditambahkan manual di akhir).
- `delivery_cost` TETAP `0` — belum ada sumber data di sisi sudomobile, di luar scope perbaikan ini.

**⚠️ BREAKING CHANGE lanjutan (2026-11-20) — Rounding**: field baru level order `grand_total` (akumulasi item + `total_service_charge` + `order_fee`, SEBELUM dibulatkan — ini yang SEBELUMNYA namanya `total_billing`) dan `rounding_amount` (selisih pembulatan, dari setting `master_branch_setting.flag_rounding`/`rounding_nearst_value` milik `branch_id`). `total_billing` SEKARANG = `grand_total + rounding_amount` (field lama TETAP ADA, tetap berarti nilai final yang ditagih — cuma nilainya sekarang bisa beda dari `grand_total` kalau branch ini pakai rounding). Formula rounding ada di bagian bawah dokumen ini.

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
        "net_price": "111930.00",
        "dpp": "100737.00",
        "tax_amount": "10073.70",
        "service_charge_id": 12,
        "service_charge_rate": "10.00",
        "service_charge_amount": "10073.70",
        "total": "120884.40",
        "promo_id": 23,
        "promo_name": "TEST PROMO CALC",
        "discount_amount": "11193.00",
        "packages": []
      }
    ],
    "sub_total": "223860.00",
    "total_tax": "20147.40",
    "total_discount": "11193.00",
    "total_service_charge": "20147.40",
    "order_fee": "10000.00",
    "grand_total": "271915.80",
    "rounding_amount": "0.00",
    "total_billing": "271915.80",
    "point_redeem_amount": 0
  }
}
```

(Contoh di atas item qty 2 + promo, TANPA package, biar breakdown promo keliatan jelas. `packages[]` tetap bisa dipakai bareng `promo_id` di request yang sama — promo cuma mempengaruhi baris item utama, sub-item package gak ikut kena diskon, lihat contoh dengan package tanpa promo di [`GET VISIT PURPOSE DETAIL.md`](../MENU/GET%20VISIT%20PURPOSE%20DETAIL.md) atau di bagian "Tervalidasi live" sebelumnya di dokumen ini.)

**Penting soal angka per item**: `net_price`/`dpp`/`tax_amount`/`service_charge_amount`/`total` itu **PER 1 UNIT** (belum dikali `qty`) — biar frontend gampang nampilin "harga satuan" di keranjang. `sub_total`/`total_tax`/`total_service_charge`/`grand_total`/`total_billing` di level ATAS itu udah di-scale `qty` (dan buat sub-item package, di-scale `qty` item induk **DIKALI** `qty` selection-nya — 1 porsi menu qty 2, tiap porsi ambil 1 varian, jadi varian-nya kehitung 2x juga). `order_fee` BEDA — nominal TETAP per order dari config, TIDAK di-scale apa pun. `rounding_amount` BEDA lagi — bukan hasil scale item sama sekali, murni selisih pembulatan `grand_total` ke kelipatan `rounding_nearst_value` terdekat.

`point_redeem_amount` (2026-09-30) — SUM `min_point_amount` dari semua promo di `use_promo_ids` yang punya syarat poin (unik per promo, bukan per baris item). Di `Calculate()` (endpoint ini) field ini **cuma informatif** (preview "bakal kepotong segini") — poinnya **belum** kepotong beneran di sini, baca-only. Poin baru BENERAN dipotong kalau order-nya disubmit lewat `Create()` — lihat [`CREATE ORDER.md`](CREATE%20ORDER.md#potong-poin-buat-promo-bersyarat-poin-min_point_amount-2026-09-30) dan [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md) (barrier #12).

## Formula (2026-11-19, replika PERSIS `OrderServices::CalculateLineItem()` PHP — POS)

**⚠️ Rename kolom (2026-11-19)**: `dpp` (lama, "sebelum diskon") → **`net_price`**. `net_dpp` (lama, "basis pajak setelah diskon") → **`dpp`**. Nama kolom `dpp` sekarang punya MAKNA BARU (basis pajak), BUKAN makna lama (sebelum diskon). Dilakukan biar konsisten sama penamaan POS (`OrderServices::CalculateLineItem()`) — `net_dpp` DIHAPUS total (DB maupun response), bukan di-rename jadi `net_dpp` baru.

Urutan: harga dilepas dari pajak+service charge GABUNGAN kalau `flag_inclusive_tax=true` — `net_price = price / (1 + (tax_rate + service_charge_rate)/100)` (kalau exclusive, `net_price = price` apa adanya) → diskon dipotong dari `net_price` (`dpp = net_price - discount_amount` — `discount_amount` hasil resolve `promo_id`, lihat [`KETENTUAN PROMO.md`](KETENTUAN%20PROMO.md); `0` kalau item gak minta promo) → pajak DAN service charge final dihitung ULANG dari `dpp` (`tax_amount = dpp * tax_rate/100`, `service_charge_amount = dpp * service_charge_rate/100`), BUKAN dari harga awal. `total` per unit = `dpp + tax_amount + service_charge_amount`.

**Kunci buat inclusive price**: pembagi BUKAN cuma `tax_rate`, tapi GABUNGAN `tax_rate + service_charge_rate` — kalau cuma dibagi tax_rate doang, `net_price` yang dihasilkan salah buat visit purpose yang punya service charge aktif. Ini detail yang gampang terlewat kalau nge-port formula POS tanpa baca source aslinya.

`service_charge_rate` diresolve dari `master_branch_visit_purpose.service_charge` (tax_id, FK `master_tax`) — SATU rate yang SAMA dipakai ke SEMUA item dalam 1 order (level visit purpose, BUKAN per-item custom kayak POS). `order_fee` dari `master_branch_visit_purpose.order_fee` — nominal rupiah TETAP, ditambahkan apa adanya ke `total_billing` di akhir, TIDAK lewat `CalculateLine()` (beda dari service charge yang otomatis masuk lewat `total` per item).

Perhitungan pakai `float64` biasa (BUKAN decimal library) — niru PERSIS cara POS (PHP float) ngitung, termasuk potensi floating-point imprecision-nya, konsisten sama prinsip `ResolveItemTax()` yang niru PERSIS behavior POS apa adanya.

Diimplementasikan di `sudomobile/backend/pricing/pricing.go` (`CalculateLine()`) — fungsi SATU-SATUNYA yang dipakai buat hitung harga/pajak di seluruh `sudomobile`, dipakai ulang juga oleh `GET VISIT PURPOSE DETAIL.md` (versi ringan, cuma `tax_amount` per unit tanpa breakdown `net_price`/`dpp` di response — service charge BELUM relevan di situ, endpoint itu murni preview menu) dan dipakai lagi sama [`POST /api/order/create-order`](CREATE%20ORDER.md) — **BUKAN 2 implementasi terpisah**, biar harga preview di keranjang gak pernah beda sama harga final pas checkout.

## Formula Rounding (2026-11-20)

Setelah `grand_total` final (sudah termasuk semua item + `total_service_charge` + `order_fee`), `rounding_amount` dihitung dari setting `master_branch_setting` milik `branch_id` di request:

```
sisa = fmod(grand_total, rounding_nearst_value)
rounding_amount = (flag_rounding == true DAN rounding_nearst_value > 0 DAN sisa != 0) ? rounding_nearst_value - sisa : 0
total_billing = grand_total + rounding_amount
```

`flag_rounding`/`rounding_nearst_value` sudah lama ada sebagai setting (migration sudocore2 030), baru dipakai transaksi sejak perubahan ini. Mode pembulatan SELALU "ke kelipatan terdekat DI ATAS" — kolom `rounding_mode` ('up'/'down') di `master_branch_setting` tidak pernah dipakai dan sudah **DIHAPUS** (2026-11-24, migration 354 sudocore2; keputusan: 1 mode saja, sama persis POS lokal — `OrderServices::RecalculateOrderTotals()`). Kalau `grand_total` sudah pas kelipatan `rounding_nearst_value` (sisa bagi `0`), `rounding_amount` TETAP `0` — tidak ikut nambah 1 kelipatan penuh. Diimplementasikan di `sudomobile/backend/pricing/pricing.go` (`ResolveRoundingConfig()`+`CalculateRoundingAmount()`), dipanggil dari `calculateOrder()` (`order_handler.go`) — SATU fungsi yang sama dipakai 4 endpoint (member app `Calculate`/`Create`, QR Order `Calculate`/`Create`), jadi rounding konsisten di semua jalur.

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

## Tervalidasi live (2026-11-19) — Service Charge & Order Fee

Data real `branch_id=51`/`visit_purpose_id=7` (`master_branch_visit_purpose.id=218`: `service_charge`=tax_id `12` rate `10.00%`, `order_fee=10000.00`, `menu_template_id=9`), item `menu_id=99` ("MENU PASTRY", harga `123123.00`, `use_tax=pb1` rate `10.00%`, inclusive) qty 1, tanpa promo:

- `net_price = 123123 / (1 + (10+10)/100) = 102602.50` (dibagi gabungan tax+SC 20%, BUKAN tax doang).
- `dpp = 102602.50` (discount 0) → `tax_amount = 10260.25`, `service_charge_amount = 10260.25`.
- `total` per item = `102602.50 + 10260.25 + 10260.25 = 123123.00` (balik ke harga asli, konsisten round-trip inclusive).
- `total_service_charge = 10260.25`, `order_fee = 10000.00` (dari config, bukan `0` lagi).
- `total_billing = 102602.50 + 10260.25 + 10260.25 + 10000 = 133123.00` — dicek manual, cocok.

Diverifikasi juga end-to-end lewat `POST /qr-order/create-order` (jalur publik, reuse `calculateOrder()` yang sama) — hasil INSERT ke `mb_order` (`total_service_charge=10260.25`, `order_fee=10000.00`) dan `mb_order_detail` (`net_price`, `dpp`, `service_charge_id/rate/amount`) persis sama dengan hasil `Calculate()` di atas. Data test dihapus setelah verifikasi.

## Tervalidasi live (2026-08-24)

**(⚠️ historis, pre-rename)** — hasil test di bawah pakai nama kolom LAMA (`dpp`/`net_dpp`) dan visit purpose TANPA service charge aktif, sebelum perubahan 2026-11-19 di atas. Dicatat tetap di sini sebagai jejak histori paket/promo, bukan field names yang masih aktif.

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
