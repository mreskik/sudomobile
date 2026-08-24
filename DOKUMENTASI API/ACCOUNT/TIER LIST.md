# Account - Tier List

Daftar **SEMUA** level tier yang terdaftar — dipakai buat render "roadmap"/"road to next tier" di app, nunjukin semua level + syarat spending-nya sekaligus. Daftarnya **sama buat semua orang** (gak ada personalisasi) — buat tau posisi member sekarang, bandingin sendiri di app ke `tier.level` dari [TIER AND SPENDING INFORMATION.md](TIER%20AND%20SPENDING%20INFORMATION.md).

```
GET /api/account/tier-list
Authorization: Bearer <token>
```

**Protected** — sengaja disamain pola sama endpoint `account/*` lain, walau isinya sendiri bukan data pribadi.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    { "level": 1, "name": "Bronze", "spending_amount": "0.00", "style_template": null },
    { "level": 2, "name": "Silver", "spending_amount": "500000.00", "style_template": null },
    { "level": 3, "name": "Gold", "spending_amount": "2000000.00", "style_template": null }
  ]
}
```

| Field | Tipe | Keterangan |
|---|---|---|
| `level` | number | Nomor tier, urut dari terendah (`ORDER BY level ASC`) |
| `name` | string | Nama tampilan tier (mis. `"Bronze"`) |
| `spending_amount` | string (numeric) | Minimal total belanja buat nyampe/pertahanin level ini — dipakai app buat hitung "kurang berapa lagi" ke level berikutnya |
| `style_template` | string, nullable | Buat tampilan (warna/badge) — struktur/isinya belum ditentukan |

List kosong `[]` kalau admin belum pernah setup [MASTER MEMBER TIER SETTING.md](../../../sudocore2/DOKUMENTASI%20API/MASTER/MASTER%20MEMBER%20TIER%20SETTING.md) sama sekali di ERP.

## Catatan

- **Beda dari [TIER AND SPENDING INFORMATION.md](TIER%20AND%20SPENDING%20INFORMATION.md)'s `tier`** — endpoint itu balikin 1 objek tier (posisi member sekarang, digabung info spending/evaluasi), endpoint ini balikin **seluruh daftar** — dipakai bareng buat kebutuhan beda: `/tier-spending` buat nampilin badge/progress di profil, `/tier-list` buat halaman "semua tier" / progress bar roadmap.
- **Sengaja gak ada `is_current`** (2026-08-21) — endpoint ini **gak nyentuh `master_member` sama sekali**, murni baca `master_member_tier_setting_detail`, biar gak ada `JOIN`/query tambahan. Nyocokin posisi member sekarang (bandingin `level` di sini ke `tier.level` dari [TIER AND SPENDING INFORMATION.md](TIER%20AND%20SPENDING%20INFORMATION.md)) itu tanggung jawab frontend, bukan backend.
- `spending_amount` di sini format string numeric mentah dari DB (`"0.00"`) — sama pola kayak field numeric lain di seluruh ekosistem ini (`balance`, `point`, dst), bukan angka/number JSON biar gak ada masalah presisi desimal.

## Tervalidasi

- **Query** (2026-08-21) — tervalidasi langsung lewat `psql` ke data live: 3 baris tier (`Bronze`/`Silver`/`Gold`) balik urut bener. `go build`/`go vet` bersih.

**⚠️ Belum tervalidasi lewat HTTP request** — belum sempat dicoba lewat request HTTP beneran (server dev butuh restart). Update bagian ini kalau udah dites.
