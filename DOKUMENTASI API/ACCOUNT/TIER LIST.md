# Account - Tier List

Daftar **SEMUA** level tier yang terdaftar (bukan cuma tier member yang lagi login) — dipakai buat render "roadmap"/"road to next tier" di app, nunjukin semua level + syarat spending-nya sekaligus, plus tandain posisi member sekarang.

```
GET /api/account/tier-list
Authorization: Bearer <token>
```

**Protected** — `member_id` dari session token, sama pola kayak [ME.md](ME.md). Isinya sendiri bukan data pribadi (daftar tier-nya sama buat semua orang), cuma `is_current` yang personal — makanya tetap butuh session biar tau harus nandain baris yang mana.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    { "level": 1, "name": "Bronze", "spending_amount": "0.00", "style_template": null, "is_current": true },
    { "level": 2, "name": "Silver", "spending_amount": "500000.00", "style_template": null, "is_current": false },
    { "level": 3, "name": "Gold", "spending_amount": "2000000.00", "style_template": null, "is_current": false }
  ]
}
```

| Field | Tipe | Keterangan |
|---|---|---|
| `level` | number | Nomor tier, urut dari terendah (`ORDER BY level ASC`) |
| `name` | string | Nama tampilan tier (mis. `"Bronze"`) |
| `spending_amount` | string (numeric) | Minimal total belanja buat nyampe/pertahanin level ini — dipakai app buat hitung "kurang berapa lagi" ke level berikutnya |
| `style_template` | string, nullable | Buat tampilan (warna/badge) — struktur/isinya belum ditentukan |
| `is_current` | bool | `true` di baris yang `level`-nya cocok sama `tier_level` member yang lagi login. **Selalu tepat 1 baris** yang `true` (kecuali member-nya punya `tier_level` yang gak ada di daftar ini — lihat "Catatan") |

List kosong `[]` kalau admin belum pernah setup [MASTER MEMBER TIER SETTING.md](../../../sudocore2/DOKUMENTASI%20API/MASTER/MASTER%20MEMBER%20TIER%20SETTING.md) sama sekali di ERP.

## Catatan

- **Beda dari [ME.md](ME.md)'s `tier`** — `/me` cuma balikin 1 objek tier (posisi member sekarang), endpoint ini balikin **seluruh daftar** — dipakai bareng buat kebutuhan beda: `/me` buat nampilin badge/status di profil, `/tier-list` buat halaman "semua tier" / progress bar roadmap.
- `is_current` bisa aja **gak ada satupun `true`** kalau `master_member.tier_level` member ini gak match level manapun yang ada di daftar (mis. admin baru aja ngehapus definisi level yang lagi ditempatin member — kasus tepi, seharusnya jarang kejadian karena replace-all di [MASTER MEMBER TIER SETTING.md](../../../sudocore2/DOKUMENTASI%20API/MASTER/MASTER%20MEMBER%20TIER%20SETTING.md) gak otomatis nge-reset `tier_level` member yang levelnya kehapus).
- `spending_amount` di sini format string numeric mentah dari DB (`"0.00"`) — sama pola kayak field numeric lain di seluruh ekosistem ini (`balance`, `point`, dst), bukan angka/number JSON biar gak ada masalah presisi desimal.

## Tervalidasi

- **Query** (2026-08-21) — tervalidasi langsung lewat `psql` ke data live: 3 baris tier (`Bronze`/`Silver`/`Gold`) balik urut bener, member `id=14` (`tier_level=1`) bakal ke-flag `is_current: true` di baris `level=1`. `go build`/`go vet` bersih.

**⚠️ Belum tervalidasi lewat HTTP request** — belum sempat dicoba lewat request HTTP beneran (server dev butuh restart). Update bagian ini kalau udah dites.
