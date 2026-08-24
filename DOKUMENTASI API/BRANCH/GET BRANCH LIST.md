# Branch - Get Branch List

```
GET /api/branch
```

**Publik** (gak butuh `Authorization`) — daftar branch yang nerima online order lewat mobile customer app, dipakai misal buat milih lokasi pickup/pengantaran sebelum member login.

Wajib header `X-App-Setting` (sama kayak semua route lain di service ini). **Gak di-scope `brand_id`** (2026-08-24, konfirmasi eksplisit) — beda dari `GET /api/banner`, endpoint ini nampilin semua branch yang online order-nya aktif, gak peduli brand.

## Request

Gak ada body/param.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 14,
      "name": "TONAKO BANDUNG",
      "address": "Jl. Asia Afrika No 10 Bandung",
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

Array kosong (`[]`) kalau belum ada branch yang online order-nya aktif — bukan error.

Diurutkan `name` ASC.

## Sumber data

```sql
SELECT
  mb.id, mb.name, mb.address,
  mbr.name AS brand_name, mbr.logo_path AS logo_brand_src,
  mb.location_coordinate,
  mbos.status AS ops_status, mbos.open_time, mbos.closed_time
FROM master_branch mb
JOIN master_branch_setting mbs ON mbs.branch_id = mb.id
LEFT JOIN master_brand mbr ON mbr.id = mb.brand_id
LEFT JOIN master_branch_ops_setting mbos ON mbos.branch_id = mb.id AND mbos.day = ?  -- hari ini, "monday".."sunday"
WHERE mbs.flag_online_service_mobile_customer = true AND mb.status = '1'
ORDER BY mb.name ASC
```

Baca langsung dari DB `sudocore2` (`sudomobile` connect ke DB yang sama, gak ada sync/bridge layer). `location_coordinate` dipecah jadi `latitude`/`longitude` di Go (`pecahLocationCoordinate()`), gak langsung diteruskan apa adanya. `flag_status_store_open` dihitung di Go (`isStoreOpenNow()`), bukan dari kolom DB manapun.

## Logic filter

- **`master_branch_setting.flag_online_service_mobile_customer = true`** — branch-nya diaktifin admin ERP buat nerima order online lewat mobile app (lihat `MASTER BRANCH.md` di sudocore2, field ini ada di object `setting` pas Create/Update branch).
- **`master_branch.status = '1'`** — branch-nya sendiri masih aktif/gak ditutup. Branch yang nonaktif gak boleh muncul di app customer walau kebetulan `flag_online_service_mobile_customer`-nya `true`.
- Field yang dibalikin **sengaja minimal** sesuai kebutuhan awal — field lain (`telp`, dll) belum diikutin, bisa ditambah belakangan kalau kebutuhan mobile app berkembang.
- `brand_name`/`logo_brand_src` (2026-08-24) — dua-duanya dari `master_brand` (`LEFT JOIN` lewat `master_branch.brand_id`), **BUKAN** kolom milik branch itu sendiri. `logo_brand_src` sempet salah ambil `master_branch.logo_header_src` di iterasi awal, udah dikoreksi. `LEFT JOIN` (bukan `JOIN`) biar branch tetep muncul kalau somehow brand-nya gak ketemu -- `brand_name`/`logo_brand_src` jadi `null` aja, bukan branch-nya ikut hilang dari list. `logo_brand_src` path lokal (`/storage/uploads/images/...`), sama pola kayak `banner_src`/`profile_photo_src` di modul lain -- `sudomobile` gak download ulang, tinggal serve dari mount storage yang sama kayak `sudocore2` (lihat `CATATAN INTERNAL.md`).
- `latitude`/`longitude` (2026-08-24) — hasil pecah `master_branch.location_coordinate` (format DB-nya 1 string `"latitude,longitude"`, contoh placeholder di form ERP: `"-6.1800448,106.9481984"`). Kalau formatnya gak sesuai (bukan persis 2 bagian dipisah koma, atau salah satu/kedua bagian bukan angka valid -- ada data test di DB yang isinya string sembarangan kayak `"tes"`), **kedua field dibalikin `null`** (bukan cuma yang gagal doang), biar app gak perlu nebak-nebak "ada koordinat tapi cuma latitude" -- selalu "ada koordinat lengkap" atau "gak ada sama sekali".
- **`status`/`open_time`/`closed_time`/`flag_status_store_open`** (2026-08-24) — jam operasional **HARI INI** (`master_branch_ops_setting`, `LEFT JOIN` filter `day = <hari ini>`), replika logic `DayShiftServices::GetOperationalHoursToday()` di POS (`posv1-laravel`) -- sengaja pola itu (bukan `GetKioskDayStatus()` yang gabung status dayshift), soalnya `sudomobile` **gak ada konsep dayshift** kayak POS (dikonfirmasi eksplisit).
  - `status` -- nilai mentah dari DB: `"open"`, `"closed"`, atau `"always_open"`. `null` kalau ops setting hari ini belum di-setting sama sekali.
  - `open_time`/`closed_time` -- jam operasional hari ini (format `"HH:MM:SS"`), `null` kalau `status` bukan `"open"` (buat `always_open`/`closed` gak relevan) atau belum di-setting.
  - `flag_status_store_open` -- **dihitung di server** (`isStoreOpenNow()`), bukan kolom DB: `always_open` → selalu `true`; `open` → `true` cuma kalau jam sekarang di antara `open_time`-`closed_time` (string compare `"HH:MM:SS"`, **belum** nanganin rentang yang nyebrang tengah malam macam `18:00`-`02:00` -- keterbatasan yang sama kayak versi POS-nya, bukan bug baru); `closed`/`null` (belum di-setting) → selalu `false`.
  - Beda dari `logo_brand_src`/`latitude`/`longitude` yang kalau gagal balikin `null`, `flag_status_store_open` **selalu** ada nilainya (`true`/`false`, gak pernah `null`) -- biar app gampang langsung dipakai buat kondisi tampilan ("Buka"/"Tutup") tanpa perlu cek null dulu.

## Tervalidasi live (2026-08-24)

Set `flag_online_service_mobile_customer = true` buat 1 branch test (`TONAKO BANDUNG`, id `14`, `brand_id=6` TONAKO) langsung via `psql` → `GET /api/branch` balikin branch itu dengan `id`/`name`/`address`/`brand_name`/`logo_brand_src` yang bener -- `brand_name: "TONAKO"` kebukti ambil dari `master_brand`, `logo_brand_src` balik `""` karena brand TONAKO kebetulan `logo_path`-nya string kosong di data dev (bukan bug, kolomnya emang bener yang dituju). Sekalian dites `location_coordinate`: di-set `"-6.1800448,106.9481984"` → `latitude: -6.1800448, longitude: 106.9481984` balik bener; di-set `"tes"` (format gak valid) → `latitude: null, longitude: null`, sesuai desain fallback aman.

**Jam operasional (4 skenario)**, data test di `master_branch_ops_setting` (baris `monday`, hari tes ini dijalanin) branch `14`:
1. `status='always_open'` → `flag_status_store_open: true`, `open_time`/`closed_time: null`.
2. `status='open'`, jam sekarang DI LUAR `open_time`-`closed_time` (di-set `01:00:00`-`02:00:00`) → `flag_status_store_open: false`.
3. `status='open'`, jam sekarang DI DALAM range (di-set `00:00:00`-`23:59:59`) → `flag_status_store_open: true`.
4. `status='closed'` → `flag_status_store_open: false`, gak peduli `open_time`/`closed_time`.

Semua 4 hasil sesuai desain. Flag & semua data test (termasuk baris `master_branch_ops_setting` yang diubah-ubah) dibalikin ke semula abis verifikasi (state semula, gak ninggalin perubahan data).
