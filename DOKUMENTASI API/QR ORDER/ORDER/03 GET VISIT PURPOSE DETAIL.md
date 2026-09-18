# QR Order - Get Visit Purpose Detail

**Status: SELESAI & tervalidasi live (2026-09-17).**

```
GET /qr-order/visit-purpose/detail?db_code=SUDO&company_code=SUDO&branch_code=SBJ&visit_purpose_code=DIN
```

**Publik** — tanpa `Authorization`, tanpa `X-App-Setting` (lihat [`KETENTUAN QR ORDER.md`](../KETENTUAN%20QR%20ORDER.md)).
Versi QR Order dari [`../MOBILE/MENU/GET VISIT PURPOSE DETAIL.md`](../../MOBILE/MENU/GET%20VISIT%20PURPOSE%20DETAIL.md):
pohon menu (category → subcategory → item) + harga + resolusi pajak + package/varian buat 1 visit
purpose di 1 branch. **Bedanya cuma cara nunjuk branch & visit purpose-nya** — di sini pakai 4 kode
identitas QR Order lewat **query param `GET`** (keputusan 2026-09-17), bukan `branch_id`/
`visit_purpose_id` di path. Isi response (tree, harga, pajak, package) **sama persis** — logic
resolusinya direncanakan **reuse fungsi yang sama** (`pricing`/`buildMenuTree()`/`fetchPackages()`),
bukan ditulis ulang.

## Request

Semua **wajib**, dikirim sebagai query param (`GET`):

| Query param | Nunjuk ke | Validasi |
| --- | --- | --- |
| `db_code` | Penanda database/tenant | Cuma dicek gak kosong — **belum dipakai/dicocokin** ke mana pun (lihat Ketentuan #1). |
| `company_code` | `master_company.code` | Exact match case-insensitive. |
| `branch_code` | `master_branch.code` | Exact match case-insensitive; branch **wajib milik company** di atas (`master_branch.company_id`), dan aktif (`status = '1'`). |
| `visit_purpose_code` | `master_visit_purpose.code` | Exact match case-insensitive; **wajib nyambung ke branch** lewat `master_branch_visit_purpose` (`branch_id` + `visit_purpose_id`, `flag_mobile_customer = true`, `is_active = true`). |

Urutan cek persis urutan tabel di atas — gagal di satu langkah langsung balik error itu, langkah
di bawahnya gak dijalanin.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "db_code": "SUDO",
    "company": { "id": 0, "code": "SUDO", "name": "PT SUDO BREW NARATAMA" },
    "branch": { "id": 59, "code": "SBJ", "name": "SUDO BARBER JGC", "address": "Jl. Boulevard Jakarta Garden City No. 1, Cakung, Jakarta Timur" },
    "visit_purpose": { "id": 1, "code": "DIN", "name": "DINE IN" },
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
                "item_description": null,
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
                        "item_description": null,
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

**Tambahan khusus QR Order** (gak ada di versi member app): blok `db_code` / `company` / `branch` /
`visit_purpose` di atas — hasil resolve 4 kode, biar FE (yang cuma megang kode dari QR) langsung
dapet `id` + nama buat dipakai di request berikutnya & ditampilin. `branch.name` pakai
`master_branch.name_qr_order` kalau diisi, fallback `master_branch.name`; `branch.address` apa
adanya dari `master_branch.address` (`null` cuma kalau kolomnya beneran `NULL` di DB — banyak
branch dev sekarang isinya string kosong `""`, bukan `NULL`, jadi tetep `""` bukan `null`, apa
adanya dari data).

**Sisanya sama persis** versi member app — `menu_template_id`, `flag_inclusive_tax`, pajak level
visit purpose (`service_charge`/`vat`/`pb1` + `_rate`), `order_fee`, dan seluruh isi `categories[]`
(item, `price` mentah, `tax_type`/`tax_id`/`tax_rate`, `package_list[]`/`menu_package_list[]`).
Penjelasan tiap field, aturan `qr_order = true`, resolusi pajak dari `master_item.use_tax`, dst —
**lihat [`../MOBILE/MENU/GET VISIT PURPOSE DETAIL.md`](../../MOBILE/MENU/GET%20VISIT%20PURPOSE%20DETAIL.md)**,
sengaja gak diulang di sini biar gak ada 2 sumber kebenaran.

- `categories: []` kalau visit purpose ini belum ada item `qr_order = true` — bukan error (sama
  kayak member app).

## Error

Semua `code: 100`, HTTP tetep `200`. Salah satu dari 4 kode gak valid → **selalu error**, gak ada
fallback (keputusan 2026-09-17):

| `message` | Kapan |
| --- | --- |
| `db_code wajib diisi` / `company_code wajib diisi` / `branch_code wajib diisi` / `visit_purpose_code wajib diisi` | Query param kosong / gak dikirim |
| `company tidak ditemukan` | `company_code` gak ada di `master_company` |
| `branch tidak ditemukan` | `branch_code` gak ada di `master_branch` |
| `branch bukan milik company ini` | Branch ketemu, tapi `master_branch.company_id` ≠ company dari `company_code` |
| `branch tidak aktif` | `master_branch.status != '1'` |
| `visit purpose tidak ditemukan` | `visit_purpose_code` gak ada di `master_visit_purpose`, ATAU ada tapi gak nyambung ke branch ini (`master_branch_visit_purpose` gak ada / `flag_mobile_customer = false` / `is_active = false`) — sengaja 1 pesan, sama semantik kayak member app |

## Catatan implementasi

- Route di `/qr-order` (di luar `/api`, lihat gotcha routing Fiber v3 di
  [`CREATE ORDER.md`](./06%20CREATE%20ORDER.md#catatan-implementasi)).
- Resolve 4 kode lewat `qrorder.Resolve()` — 3 query bertahap (company → branch → visit purpose),
  bukan 1 `JOIN` raksasa, biar pesan error-nya bisa dibedain per langkah persis tabel di atas.
  `qrorder.Context` juga bawa `CompanyName`/`BranchName` (udah `COALESCE(name_qr_order, name)`)/
  `BranchAddress`/`VisitPurposeName` sekalian — gak nambah round-trip, query-nya emang udah narik
  baris itu.
- Handler-nya hidup di package `visitpurpose` yang sama kayak member app (bukan package `qrorder`
  terpisah) — logic tree menu/harga/pajak/package DIEKSTRAK jadi `resolveVisitPurposeDetail()`
  (dipisah dari `GetDetail()` member app 2026-09-17) biar dipakai bareng TANPA duplikasi query
  ~20 baris. Bentuk response-nya beda (blok `db_code`/`company`/`branch`/`visit_purpose` di QR,
  `visit_purpose_id` polos di member app), jadi bukan share struct, cuma share resolusi datanya.

## Tervalidasi live (2026-09-17)

Branch 51 (`SBE`)/company `SUDO`/visit purpose 7 (`ASD`) — data yang sama dipakai buat tes
Create Order & Payment Status:

- Identitas bener → `code: 0`, blok `company`/`branch`/`visit_purpose` kebukti bener
  (`{"id":0,"code":"SUDO","name":"PT SUDO BREW NARATAMA"}` dst), `categories[]` isinya identik
  struktur & data sama versi member app (`item_id: 109`, `package_list[]` lengkap termasuk
  sub-item-nya).
- 4 kode identitas: `db_code` kosong → `"db_code wajib diisi"`. `company_code` salah →
  `"company tidak ditemukan"`. `branch_code` salah → `"branch tidak ditemukan"`. `branch_code`
  valid tapi `company_code` company lain → `"branch bukan milik company ini"`.
  `visit_purpose_code` salah → `"visit purpose tidak ditemukan"`.
- Semua 4 kode **lowercase** → tetap sukses, response identik (case-insensitive kebukti jalan).

Endpoint ini `GET` doang / baca-only, gak ada data yang perlu dibersihin setelahnya. `go
build`/`go vet` bersih.
