# Branch - Get Visit Purpose List

```
GET /api/branch/:branch_id/visit-purpose
```

**Publik** (gak butuh `Authorization`) — daftar visit purpose yang dibolehin muncul di mobile customer app buat 1 branch. Mirror [`KIOSK BRANCH VISIT PURPOSE.md`](../../../POS/posv1-laravel/DOKUMENTASI%20API/KIOSK/KIOSK%20BRANCH%20VISIT%20PURPOSE.md) di POS, dengan 2 beda utama:

- Filter pakai `flag_mobile_customer` (bukan `flag_kiosk`).
- `branch_id` **eksplisit di URL** (`:branch_id`) — beda dari Kiosk yang implisit (1 install POS = 1 branch doang). `sudomobile` ngelayanin banyak branch sekaligus (lihat [`GET BRANCH LIST.md`](GET%20BRANCH%20LIST.md)), jadi wajib nentuin branch mana yang dimaksud.

## Request

`:branch_id` — id branch (dari `GET /api/branch`). Gak ada body.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    { "id": 200, "visit_purpose_id": 1, "visit_purpose_name": "DINE IN" },
    { "id": 203, "visit_purpose_id": 2, "visit_purpose_name": "TAKEAWAY" }
  ]
}
```

- `id` — id baris `master_branch_visit_purpose` itu sendiri. **Bukan** yang dipakai buat manggil endpoint detail nanti.
- `visit_purpose_id` — FK ke `master_visit_purpose.id`. **Ini** yang bakal dipakai buat endpoint detail (belum digarap — lihat "Status" di bawah).
- `visit_purpose_name` — `master_visit_purpose.name`.

`branch_id` yang gak valid (bukan angka) → `{ "code": 100, "message": "branch_id tidak valid" }`. Branch yang gak punya visit purpose sama sekali (atau `branch_id` gak ketemu) → array kosong `[]`, bukan error.

## Sumber data

```sql
SELECT bvp.id, bvp.visit_purpose_id, vp.name AS visit_purpose_name
FROM master_branch_visit_purpose bvp
JOIN master_visit_purpose vp ON vp.id = bvp.visit_purpose_id
WHERE bvp.branch_id = ? AND bvp.flag_mobile_customer = true
	AND bvp.is_active = true AND vp.is_active = true
ORDER BY vp.name ASC
```

Baca langsung dari DB `sudocore2` (`sudomobile` connect ke DB yang sama, gak ada sync/bridge layer kayak POS↔APIANDORDER).

`is_active` di **kedua** tabel (`master_branch_visit_purpose` DAN `master_visit_purpose`) dicek eksplisit — beda dari versi Kiosk POS yang gak eksplisit ngecek ini (POS-nya udah "aman" duluan karena sync-nya emang cuma narik baris yang masih relevan; `sudomobile` baca langsung ke ERP jadi perlu jaga sendiri).

## Status

List (halaman ini) selesai. **Detail** ([`GET VISIT PURPOSE DETAIL.md`](GET%20VISIT%20PURPOSE%20DETAIL.md)) digarap bertahap (2026-08-24) — Tahap 1 (tree menu + harga, tanpa pajak/package) udah jalan, Tahap 2 (pajak) & Tahap 3 (package/varian) masih nyusul. Lihat dokumentasi Detail buat rencana lengkapnya.

## Tervalidasi live (2026-08-24)

Dites langsung ke data real branch `14` (TONAKO BANDUNG) — ada 4 baris `master_branch_visit_purpose`, 2 di antaranya `flag_mobile_customer=true` (`DINE IN`, `TAKEAWAY`), 2 lainnya `false` (`OJEK ONLINE`, `LESEHAN`). Response cuma balikin 2 yang `true`, sesuai desain. Dites juga `branch_id` yang gak punya data (`999999`) → `[]`, dan `branch_id` non-angka (`abc`) → `"branch_id tidak valid"`. Gak ada data test yang perlu dibersihin (murni baca data existing).
