# QR Order - Get Visit Purpose List

**Status: SELESAI & tervalidasi live (2026-09-18).**

```
GET /qr-order/visit-purpose/list?db_code=SUDO&company_code=SUDO&branch_code=SBJ
```

**Publik** — tanpa `Authorization`, tanpa `X-App-Setting`. Daftar visit purpose yang valid buat 1
branch, dipakai customer buat **milih visit purpose** (dine in/takeaway/dst) — langkah terakhir
sebelum punya 4 kode lengkap & bisa lanjut ke endpoint transaksional/detail (Create, Calculate,
Payment Status, Order Detail, Get Visit Purpose Detail, Get Payment Method List). Dijalanin abis
[`GET BRANCH LIST.md`](./01%20GET%20BRANCH%20LIST.md) di alur "QR company doang" — lihat
[`KETENTUAN QR ORDER.md`](../KETENTUAN%20QR%20ORDER.md#identitas-request-bare-minimum--disepakati-2026-09-17-tiered-2026-09-18).
Versi QR Order dari `GetList()` member app
([`../MOBILE/MENU/`](../../MOBILE/MENU/) — endpoint `GET /api/branch/:branch_id/visit-purpose`,
sama handler `visitpurpose_handler.go`), **DI-SCOPE lewat `branch_code`** (bukan `:branch_id` di
path).

## Request

3 kode identitas wajib di query param — `db_code` + `company_code` + `branch_code` (belum butuh
`visit_purpose_code`, endpoint ini justru buat nemuin itu), lewat `qrorder.ResolveBranch()`. Gak
ada body.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    { "id": 7, "code": "ASD", "name": "asd" }
  ]
}
```

- `id` — `master_visit_purpose.id`, VISIT PURPOSE-nya LANGSUNG (**BUKAN** id baris
  `master_branch_visit_purpose` kayak versi member app — QR Order gak pernah butuh id baris join
  itu, cuma identitas visit purpose doang, jadi bentuk response-nya disederhanain).
- `code` — `master_visit_purpose.code`, dipakai jadi `visit_purpose_code` di endpoint
  transaksional/detail. Satu-satunya cara QR Order nunjuk visit purpose.
- `name` — `master_visit_purpose.name`.

Array kosong (`[]`) kalau branch itu belum ada visit purpose yang dibuka buat mobile customer —
bukan error. Diurutkan `name` ASC.

## Error

Cuma error 3 kode identitas pertama — tabel lengkapnya di
[`GET VISIT PURPOSE DETAIL.md`](./03%20GET%20VISIT%20PURPOSE%20DETAIL.md#error) (baris #1-3, `visit
purpose tidak ditemukan` di baris #4 gak relevan di sini karena visit_purpose_code emang gak
diminta). Semua `code: 100`, HTTP `200`.

## Catatan implementasi

Filter **PERSIS SAMA** `GetList()` member app (`visitpurpose_handler.go`):
`bvp.flag_mobile_customer = true AND bvp.is_active = true AND vp.is_active = true`. Query & struct
(`qrVisitPurposeListItem`) **TERPISAH** dari punya member app (bukan diekstrak) — `id` yang
dipindah ke `visit_purpose_id` langsung + `code` yang ditambah bikin bentuknya beda, pola
konsisten sama endpoint QR Order lain. Handler-nya (`visitpurpose_qr_list_handler.go`) hidup di
package `visitpurpose` yang sama kayak Get Visit Purpose Detail (`QRHandler` interface nambah
method `GetList` di file yang sama, `visitpurpose_qr_handler.go`).

`qrorder.ResolveBranch()` — resolver BARU (2026-09-18). Lihat "Riwayat" di
[`KETENTUAN QR ORDER.md`](../KETENTUAN%20QR%20ORDER.md) buat detail refactor `qrorder`.

## Tervalidasi live (2026-09-18)

Branch 51 (`SBE`)/company `SUDO` — data yang sama dipakai endpoint QR Order lain:
- Identitas bener → `[{"id":7,"code":"ASD","name":"asd"}]`, sama visit purpose yang dipakai
  [`CREATE ORDER.md`](./06%20CREATE%20ORDER.md) dst.
- `branch_code` kosong → `"branch_code wajib diisi"`. `branch_code` salah →
  `"branch tidak ditemukan"`.
- Semua 3 kode lowercase → tetap sukses, hasil identik.

Endpoint ini baca-only, gak ada data yang perlu dibersihin.
