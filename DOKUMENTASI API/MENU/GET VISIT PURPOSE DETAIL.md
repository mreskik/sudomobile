# Branch - Get Visit Purpose Detail

```
GET /api/branch/:branch_id/visit-purpose/:visit_purpose_id
```

**Publik** (gak butuh `Authorization`) — pohon menu (category → subcategory → item) + harga + resolusi pajak + package/varian buat 1 visit purpose di 1 branch. **TAHAP 3 (final, 2026-08-24)** dari rencana bertahap — lihat "Status" di bawah.

Konsep mirip [`KIOSK BRANCH VISIT PURPOSE DETAIL.md`](../../../POS/posv1-laravel/DOKUMENTASI%20API/KIOSK/KIOSK%20BRANCH%20VISIT%20PURPOSE%20DETAIL.md) di POS, TAPI querynya ditulis ulang dari nol — versi POS baca tabel lokal `mr_*` (hasil sync dari ERP), versi ini baca **langsung skema ERP** (`master_item`/`master_pricelist`/`master_tax`/dst) karena `sudomobile` connect ke DB `sudocore2` yang sama, gak ada tabel `mr_*`. Logic resolusi pajaknya sendiri **direplika persis** dari `MenuServices.php` (POS) — bukan diinterpretasi ulang dari nol, lihat "Catatan penting".

## Request

`:branch_id` — id branch (dari `GET /api/branch`). `:visit_purpose_id` — dari `GET /api/branch/:branch_id/visit-purpose` (field `visit_purpose_id`, **bukan** `id`).

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "visit_purpose_id": 7,
    "menu_template_id": 9,
    "flag_inclusive_tax": true,
    "service_charge": 12,
    "service_charge_rate": "10.00",
    "vat": 12,
    "vat_rate": "10.00",
    "pb1": 12,
    "pb1_rate": "10.00",
    "order_fee": "10000.00",
    "categories": [
      {
        "category_id": 34,
        "category_name": "MENU PASTRY",
        "subcategories": [
          {
            "subcategory_id": 19,
            "subcategory_name": "BARISTA COFFEE",
            "icon_src": "",
            "banner_src": "",
            "items": [
              {
                "item_id": 109,
                "item_code": "MP-BC-109",
                "item_name": "MENU PASTRY",
                "image_src": "",
                "icon_src": null,
                "price": "123123.00",
                "tax_type": "pb1",
                "tax_id": 12,
                "tax_rate": "10.00",
                "package_list": [
                  {
                    "package_id": 18,
                    "package_name": "ADITIONAL",
                    "min_qty": 1,
                    "max_qty": 21,
                    "menu_package_list": [
                      {
                        "menu_package_id": 31,
                        "item_id": 97,
                        "item_name": "HOT OKINAWA LATTE",
                        "price": "15000.00",
                        "icon_src": null,
                        "tax_type": "pb1",
                        "tax_id": 12,
                        "tax_rate": "10.00",
                        "default_item": false
                      }
                    ]
                  }
                ]
              }
            ]
          }
        ]
      }
    ]
  }
}
```

**Level visit purpose (top-level, sekali per response, BUKAN per item):**

- `menu_template_id` — istilah ERP-nya `master_pricelist.id` (Go: `MenuTemplateModel`) -- info doang, gak dipakai manggil apa-apa lagi.
- `flag_inclusive_tax` — dari `master_pricelist.inclusive_price`. **Satu nilai buat semua item** di response ini (beda dari POS yang taro field ini per item -- di sini sengaja disederhanakan jadi 1 di atas karena nilainya emang gak pernah beda antar item dalam 1 `menu_template`).
- `service_charge`/`vat`/`pb1` — `tax_id` (FK `master_tax`) milik visit purpose ini, apa adanya dari `master_branch_visit_purpose`. `null` kalau visit purpose-nya emang gak nyetel tax_id itu.
- `service_charge_rate`/`vat_rate`/`pb1_rate` — rate (`master_tax.rate`) hasil lookup dari `tax_id` di atas. `null` kalau `tax_id`-nya `null` **atau `0`** (dua-duanya dianggap "gak ada pajak jenis itu"), atau `tax_id`-nya gak ketemu di `master_tax`.
- `order_fee` — apa adanya dari `master_branch_visit_purpose.order_fee`.
- ⚠️ **`service_charge`/`service_charge_rate` SEKADAR INFO MENTAH** — belum ada logic yang beneran ngitung/nerapin service charge ke harga di endpoint ini (atau di manapun yang ketemu riset kode POS -- kemungkinan ini gap yang juga ada di POS). **Jangan diasumsikan** harga final udah termasuk service charge.

**Level item:**

- `tax_type` — apa adanya dari `master_item.use_tax` (string kosong kalau item-nya emang gak di-set).
- `tax_id` — hasil resolusi: `tax_type == "vat"` → pakai `vat` visit purpose, `tax_type == "pb1"` → pakai `pb1` visit purpose, selain itu (termasuk kosong, atau typo) → `null`.
- `tax_rate` — lookup `master_tax.rate` dari `tax_id` di atas, `null` kalau `tax_id`-nya `null`.
- `price` — **masih harga mentah** dari `master_pricelist_detail.price`, **belum** dihitung inclusive/exclusive-nya (itu tanggung jawab sisi konsumen/cart nanti, pakai `flag_inclusive_tax`+`tax_rate` di atas -- endpoint ini cuma nyediain bahan mentahnya, gak ngitung `dpp`/`net_dpp`/`total`).
- `package_list` — array **kosong `[]`** kalau item-nya emang gak punya package (mayoritas item). Item yang punya package (customer wajib/boleh milih sub-item dari 1+ grup, misal grup "VARIAN" pilih 1 dari beberapa varian rasa) bakal keisi.

**Level package group** (`package_list[]`):

- `package_id` — id `master_item_package_group`. `package_name` — namanya (misal `"VARIAN"`, `"ADITIONAL"`).
- `min_qty`/`max_qty` — batas jumlah sub-item yang wajib/boleh dipilih dari grup ini. Apa adanya dari DB, endpoint ini **gak validasi** pemilihan customer (itu tanggung jawab endpoint order/cart nanti).

**Level sub-item package** (`menu_package_list[]`):

- `menu_package_id` — id `master_item_package_detail`, dipakai buat ngerujuk pilihan spesifik ini pas order nanti (bukan `item_id`).
- `item_id`/`item_name` — sub-item ini **beneran baris `master_item`** sendiri (lewat `item_conversion_detail_id`), bukan data fiktif nempel di package.
- `price` — **BUKAN** dari `master_pricelist_detail` kayak item utama, basisnya `master_item_package_detail.price` (harga/surcharge KHUSUS package, konvensi ERP: `0` = "termasuk gratis" kalau dipilih, bukan berarti gagal/error). **Update (2026-08-26)**: sekarang bisa BEDA per `menu_template_id` — kalau `master_item_package_detail.flag_all_menu_template = false` **dan** ada baris `master_item_package_detail_menu_template` yang cocok buat `menu_template_id` visit purpose ini, harga override itu yang dipakai; kalau `flag_all_menu_template = true` **atau** gak ketemu override-nya, tetap fallback ke `master_item_package_detail.price` (harga di atas). Resolusinya di SQL (`FetchPackages()`, `pricing.go`) — `CASE WHEN flag_all_menu_template THEN price ELSE COALESCE(override.price, price) END`, satu query, bukan lookup terpisah.
- `default_item` — dari `master_item_package_detail.default_item` (2026-08-26), nandain sub-item mana yang "pre-selected" secara default pas customer buka package group ini. Passthrough apa adanya, gak ada logic tambahan di sini — konsumen (FE/app mobile) yang mutusin mau dipakein buat pre-check atau enggak.
- `tax_type`/`tax_id`/`tax_rate` — diresolve **PERSIS** pakai fungsi yang sama kayak item utama (`resolveItemTax()`), dari `use_tax` milik sub-item itu SENDIRI (bukan diwarisin dari item utama/parent) — kebetulan di contoh di atas sama-sama `pb1` karena datanya emang gitu, tapi bisa beda kalau sub-item-nya punya `use_tax` beda dari parent.

Kondisi lain (kosong, gak ketemu, error validasi param) sama kayak sebelumnya — lihat bagian bawah dokumen ini.

- `categories`/`subcategories` bisa `[]` kalau visit purpose ini emang belum ada item yang di-set buat channel mobile (`qr_order`, lihat "Catatan penting") -- bukan error.
- `subcategory_id`/`subcategory_name`/`icon_src`/`banner_src` bisa `null` kalau itemnya gak punya subcategory (`master_item.item_subcategory` NULL) -- item-nya tetep muncul, cuma "nempel" di array `subcategories` dengan subcategory-nya `null`.
- Kalau `visit_purpose_id` gak ketemu (salah, atau `flag_mobile_customer`-nya `false`, atau `is_active`-nya `false`) → `{ "code": 100, "message": "visit purpose tidak ditemukan" }`.
- `branch_id`/`visit_purpose_id` yang bukan angka → `{ "code": 100, "message": "branch_id tidak valid" }` / `{ "code": 100, "message": "visit_purpose_id tidak valid" }`.

## Sumber data

**1. Resolve config visit purpose (`menu_template_id` + 3 tax_id + `order_fee` + `inclusive_price`, 1 query gabungan):**
```sql
SELECT bvp.menu_template_id, bvp.service_charge, bvp.vat, bvp.pb1, bvp.order_fee,
  COALESCE(mp.inclusive_price, false) AS inclusive_price
FROM master_branch_visit_purpose bvp
LEFT JOIN master_pricelist mp ON mp.id = bvp.menu_template_id
WHERE bvp.branch_id = ? AND bvp.visit_purpose_id = ? AND bvp.flag_mobile_customer = true AND bvp.is_active = true
```

**2. Resolve rate 3 tax_id itu sekaligus (1 query, bukan 3x round-trip):**
```sql
SELECT id, rate FROM master_tax WHERE id IN (?)  -- cuma tax_id yang bukan null/0
```

**3. Tree menu + harga + `use_tax` mentah per item:**
```sql
SELECT
  mic.id AS category_id, mic.name AS category_name,
  misc.id AS subcategory_id, misc.name AS subcategory_name,
  misc.icon_src AS subcategory_icon_src, misc.banner_src AS subcategory_banner_src,
  mi.id AS item_id, mi.item_code, mi.item_name,
  mi.image AS image_src, mi.icon_src AS item_icon_src,
  mpd.price, mi.use_tax
FROM master_pricelist_detail mpd
JOIN master_item_conversion_detail micd ON micd.id = mpd.item_conversion_detail_id
JOIN master_item mi ON mi.id = micd.item_id
JOIN master_item_category mic ON mic.id = mi.item_category
LEFT JOIN master_item_sub_category misc ON misc.id = mi.item_subcategory
WHERE mpd.menu_template_id = ? AND COALESCE(mpd.is_deleted, false) = false AND mpd.qr_order = true
  AND mi.item_status = '1'
ORDER BY mic.name ASC, misc.name ASC, mi.item_name ASC
```

Resolusi `tax_id`/`tax_rate` per item dihitung di **Go** (`resolveItemTax()`), bukan di SQL — pakai `use_tax` mentah dari query 3 + config dari query 1 + map rate dari query 2. Tree-nya juga dirakit di Go (`buildMenuTree()`), sama kayak Tahap 1.

**4. Package -- 1 query BATCH buat SEMUA item_id yang ada di tree sekaligus** (bukan per-item, hindari N+1):
```sql
SELECT
  mip.item_id AS parent_item_id,
  mipg.id AS package_id, mipg.name AS package_name, mipg.min_qty, mipg.max_qty,
  mipd.id AS menu_package_id, mipd.price,
  submi.id AS sub_item_id, submi.item_name AS sub_item_name,
  submi.icon_src AS sub_item_icon_src, submi.use_tax AS sub_item_use_tax
FROM master_item_package mip
JOIN master_item_package_group mipg ON mipg.item_package_id = mip.id
JOIN master_item_package_detail mipd ON mipd.package_group_id = mipg.id
JOIN master_item_conversion_detail submicd ON submicd.id = mipd.item_conversion_detail_id
JOIN master_item submi ON submi.id = submicd.item_id
WHERE mip.item_id IN (?)
ORDER BY mip.item_id ASC, mipg.id ASC, mipd.id ASC
```

Hasilnya map `item_id → []packageGroup` (`fetchPackages()`), dipasang ke item yang cocok pas `buildMenuTree()` ngerakit tree. Item yang gak ada entrinya di map ini dikasih `package_list: []` (bukan `null`).

## Catatan penting

- **Resolusi pajak per item DIREPLIKA PERSIS dari `MenuServices.php` (POS)**, bukan diinterpretasi ulang dari nol -- ini hasil riset kode dulu, bukan tebakan:
  - `tax_type` item itu **`master_item.use_tax`**, **BUKAN** `master_item_category.tax_type`. Kolom `tax_type` di kategori itu ADA di skema (`CHECK` constraint `'pb1'`/`'vat'`), tapi ternyata **gak dipakai** di logic pajak manapun di seluruh codebase (sudocore2/APIANDORDER/POS) -- cuma field CRUD nganggur. Kalau nemu kode lain yang baca kolom itu buat nentuin pajak, itu salah/gak konsisten sama sumber aslinya.
  - `use_tax` harus **persis** `"vat"` atau `"pb1"` (case-sensitive, gak ada trim spasi). Data real kebukti ada yang isinya `"pb 1"` (ada spasi, typo) — itu **gak** dianggap `"pb1"`, item-nya jadi gak kena pajak. Ini bukan bug yang perlu dibenerin di endpoint ini, ini PERSIS behavior POS (data kotor emang ujungnya gitu, bukan urusan endpoint buat "cerdas" nebak maksud admin).
  - `tax_id` (dari visit purpose) yang `0` **atau** `NULL` dua-duanya dianggap "gak ada pajak jenis itu" -- walau `use_tax`-nya cocok.
- **`qr_order = true`** (2026-08-24, konfirmasi eksplisit) — `master_pricelist_detail` cuma punya flag channel `pos`/`qr_order`, **gak ada** `mobile_customer` yang eksplisit. `qr_order` dipilih mewakili "channel self-order customer" (mobile termasuk situ, bukan `pos` yang buat kasir) -- item yang gak di-flag `qr_order` gak akan pernah muncul di endpoint ini, walau kepake di POS/kiosk.
- **Bug ketemu & dibenerin (2026-08-24, Tahap 1)**: kolom `master_pricelist_detail.is_deleted` **NULLABLE**, dan di data real isinya `NULL` (bukan literal `false`) buat baris yang emang gak dihapus. Filter awal `mpd.is_deleted = false` gak match `NULL` di SQL — semua baris ke-exclude, tree selalu kosong. Dibenerin jadi `COALESCE(mpd.is_deleted, false) = false`.
- **Item tanpa kategori GAK muncul sama sekali** — `master_item_category` di-`JOIN` (bukan `LEFT JOIN`), karena tiap item emang wajib punya category.
- **Package cuma 2 level, gak recursive** — sub-item package (`menu_package_list[]`) gak pernah punya package-nya sendiri di response ini, walau secara skema (FK) sebenernya gak ada yang ngelarang 1 item punya package DAN sekaligus jadi sub-item package lain. Riset kode (sudocore2/APIANDORDER/POS) gak nemu satupun tempat yang nge-resolve level ke-2, jadi endpoint ini pun sengaja berhenti di 1 level (item utama → package group → sub-item), konsisten sama semua kode lain yang udah ada.
- **`master_item_category.tax_type` juga gak relevan di sini** (sama kayak item utama) — sub-item package pajaknya dari `master_item.use_tax` miliknya sendiri, bukan diwarisin dari mana pun.

## Status

Ini **Tahap 3, TERAKHIR** dari rencana bertahap — semua selesai:
1. ✅ Tree menu dasar + harga.
2. ✅ Resolusi pajak (`service_charge`/`vat`/`pb1`/`order_fee` level visit purpose + `tax_type`/`tax_id`/`tax_rate` per item, `flag_inclusive_tax` level template).
3. ✅ Package/varian (`package_list`/`menu_package_list` per item, pajak sub-item diresolve independen) -- **selesai, halaman ini**.

`price` di semua level (item utama maupun sub-item package) masih harga **mentah** — belum dihitung inclusive/exclusive jadi angka final bayar (itu tanggung jawab endpoint order/cart, di luar scope endpoint menu-browsing ini).

## Tervalidasi live (2026-08-24)

**Tahap 1**: Dites ke data real (`branch_id=51`/`visit_purpose_id=7`, `menu_template_id=9`) → 1 category/1 subcategory/1 item bener. Item lain di template yang sama dengan `qr_order=true` tapi `item_status='0'` (nonaktif) kebukti bener ke-filter. Skenario menu kosong (`branch_id=14`/`visit_purpose_id=1`) → `categories: []`; visit purpose gak ketemu (`visit_purpose_id=999`) → pesan error bener.

**Tahap 2**: Data real visit purpose `id=204` (`branch_id=51`/`visit_purpose_id=7`) punya `service_charge=vat=pb1=12` (semua nunjuk `tax_id` yang sama, rate `10.00`), `menu_template.inclusive_price=true`. Response balikin `flag_inclusive_tax: true`, `service_charge_rate`/`vat_rate`/`pb1_rate` semua `"10.00"` bener. Item `109` (`use_tax='pb1'` asli) → `tax_type: "pb1", tax_id: 12, tax_rate: "10.00"`, sesuai. Diuji 2 cabang lagi dengan ubah sementara `master_item.use_tax` item itu: `'vat'` → `tax_type: "vat", tax_id: 12` bener (kebetulan sama karena `vat`==`pb1` di data ini); `''` (kosong) → `tax_type: "", tax_id: null, tax_rate: null` bener, item tetep muncul tapi gak kena pajak. Data `use_tax` dibalikin ke `'pb1'` abis tes. Gak ada data yang perlu dibersihin selain itu (murni baca data existing + 1 kolom yang diubah-balikin).

**Tahap 3**: Kebetulan item `109` (yang udah dipake tes dari Tahap 1) beneran punya package di data real — `package_group_id=18` ("ADITIONAL", `min_qty=1`/`max_qty=21`), 1 sub-item (`HOT OKINAWA LATTE`, `item_id=97`, `use_tax='pb1'`). Response balikin `package_list` bener persis sesuai data DB, termasuk `menu_package_id=31` (id `master_item_package_detail`), `price="15000.00"` (harga package, BUKAN dari pricelist), dan pajak sub-item ke-resolve bener (`tax_type: "pb1", tax_id: 12, tax_rate: "10.00"`, sama logic kayak item utama). Gak ada data yang perlu dibersihin (murni baca data existing, gak ada yang diubah-balikin kali ini).
