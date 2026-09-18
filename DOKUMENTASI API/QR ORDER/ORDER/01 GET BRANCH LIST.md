# QR Order - Get Branch List

**Status: SELESAI & tervalidasi live (2026-09-18).**

```
GET /qr-order/branch-list?db_code=SUDO&company_code=SUDO
```

**Publik** — tanpa `Authorization`, tanpa `X-App-Setting`. Daftar branch di 1 company, dipakai
customer buat **milih branch** sebelum lanjut ke [`GET VISIT PURPOSE LIST.md`](./02%20GET%20VISIT%20PURPOSE%20LIST.md)
— skenario QR yang cuma encode `db_code`+`company_code` (QR generik outlet/website), BUKAN QR per
meja yang udah encode 4 kode lengkap (itu langsung ke endpoint transaksional, gak perlu lewat sini
sama sekali). Versi QR Order dari
[`../MOBILE/MENU/GET BRANCH LIST.md`](../../MOBILE/MENU/GET%20BRANCH%20LIST.md), **DI-SCOPE 1
company** (beda dari versi member app yang sengaja lintas company/brand).

## Request

2 kode identitas wajib di query param — **CUMA** `db_code` + `company_code` (belum butuh
`branch_code`/`visit_purpose_code`, lihat [`KETENTUAN QR ORDER.md`](../KETENTUAN%20QR%20ORDER.md#identitas-request-bare-minimum--disepakati-2026-09-17-tiered-2026-09-18)),
lewat `qrorder.ResolveCompany()`. Gak ada body.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 14,
      "code": "TB",
      "name": "TONAKO BANDUNG",
      "address": "Jl. Asia Afrika No 10 Bandung",
      "brand_id": 6,
      "brand_name": "TONAKO",
      "logo_brand_src": "/storage/uploads/images/xxxxx.png",
      "latitude": -6.1800448,
      "longitude": 106.9481984,
      "status": "always_open",
      "open_time": null,
      "closed_time": null,
      "flag_status_store_open": true
    }
  ]
}
```

Sama persis versi member app — lihat
[`../MOBILE/MENU/GET BRANCH LIST.md`](../../MOBILE/MENU/GET%20BRANCH%20LIST.md#response) buat
penjelasan tiap field (`latitude`/`longitude` hasil pecah `location_coordinate`, `status`/
`open_time`/`closed_time`/`flag_status_store_open` dari jam operasional **hari ini** +
`heartbeat.IsOnline()`). **Ditambah `code`** (`master_branch.code`, satu-satunya field baru) —
dipakai jadi `branch_code` di [`GET VISIT PURPOSE LIST.md`](./02%20GET%20VISIT%20PURPOSE%20LIST.md) &
endpoint transaksional lainnya. Array kosong (`[]`) kalau company itu gak punya branch yang
online-order-nya aktif — bukan error. Diurutkan `name` ASC.

## Error

Cuma error `db_code`/`company_code` — tabel lengkapnya di
[`GET VISIT PURPOSE DETAIL.md`](./03%20GET%20VISIT%20PURPOSE%20DETAIL.md#error) (baris #1-2 doang yang
relevan di sini). Semua `code: 100`, HTTP `200`.

## Catatan implementasi

Filter **PERSIS SAMA** [`../MOBILE/MENU/GET BRANCH LIST.md`](../../MOBILE/MENU/GET%20BRANCH%20LIST.md)
(`master_branch_setting.flag_online_service_mobile_customer = true AND master_branch.status = '1'`),
**DITAMBAH** `mb.company_id = ?` (scope company dari `qrorder.ResolveCompany()`). Query & struct
(`qrBranchListRow`/`qrBranchListItem`) **TERPISAH** dari punya member app (bukan diekstrak) —
kolom `code` yang ditambah & `WHERE company_id` yang beda bikin gak worth digabung, pola konsisten
sama endpoint QR Order lain (`qrOrderDetailHeader` dst). Yang DIPAKAI BARENG: helper
`IsStoreOpenNow()`/`pecahLocationCoordinate()` (fungsi package-level yang sama, dipanggil apa
adanya, bukan diduplikasi). Handler-nya (`branch_qr_handler.go`) hidup di package `branch` yang
sama kayak member app.

`qrorder.ResolveCompany()` — resolver BARU (2026-09-18), sibling `ResolveBranch()`/`Resolve()`
yang udah ada, sama-sama di package `qrorder`. Lihat "Riwayat" di
[`KETENTUAN QR ORDER.md`](../KETENTUAN%20QR%20ORDER.md) buat detail refactor-nya (Go struct
embedding, endpoint LAMA gak ada yang berubah kodenya).

## Tervalidasi live (2026-09-18)

Company `SUDO` (`SUDO BREW NARATAMA`):
- Identitas bener → 3 branch balik (`SBE`/`SUDO BREW - EVENT`, `TCG`/`TONAKO - CEMPAKA PUTIH`,
  `TB`/`TONAKO BANDUNG`), `code`/`id`/`name`/`address`/`brand_name` semua bener, diurutkan nama.
- `db_code` kosong → `"db_code wajib diisi"`. `company_code` salah → `"company tidak ditemukan"`.

Endpoint ini baca-only, gak ada data yang perlu dibersihin.
