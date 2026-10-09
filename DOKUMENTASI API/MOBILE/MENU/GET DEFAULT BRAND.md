# Brand - Get Default Brand

```
GET /api/brand/default
```

**Publik** (gak butuh `Authorization`) — brand aktif langsung dari header `X-App-Setting` (`brand_id`, lihat `middleware/app_setting.go`), bukan pilihan user. `brand_id` itu udah tetap per app-instance/build, endpoint ini cuma nerjemahin id itu jadi `id`+`name` yang siap ditampilin (mis. header/splash screen), gak perlu mobile app hardcode nama brand sendiri.

Wajib header `X-App-Setting` (sama kayak semua route lain di service ini).

## Request

Gak ada body/param — `brand_id` diambil dari `X-App-Setting`.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 6,
    "name": "TONAKO",
    "brand_color": "#ff6600",
    "primary_color": "#1a1a1a",
    "secondary_color": "#f5f5f5"
  }
}
```

`brand_color`/`primary_color`/`secondary_color` (2026-10-09) — warna branding buat theming di mobile app (mis. warna tombol/aksen/header), ganti dari `theme_color` tunggal yang lama di sudocore2 (dihapus total, migration `263` — lihat `MASTER BRAND.md` sudocore2). String bebas (biasanya hex `#rrggbb`, tidak divalidasi formatnya), **string kosong `""`** kalau brand itu belum pernah diisi warnanya (bukan `null` — nullable di DB di-`COALESCE` ke string kosong, konsisten sama field string lain di project ini).

## Sumber data

```sql
SELECT id, name,
COALESCE(brand_color, '') as brand_color,
COALESCE(primary_color, '') as primary_color,
COALESCE(secondary_color, '') as secondary_color
FROM master_brand WHERE id = ?  -- brand_id dari X-App-Setting
```

Baca langsung dari DB `sudocore2` (`sudomobile` connect ke DB yang sama, gak ada sync/bridge layer).

## Logic

`brand_id` di `X-App-Setting` udah divalidasi eksistensinya di `middleware.AppSetting` (`SELECT COUNT(*) FROM master_brand WHERE id = ?`) sebelum request nyampe ke handler manapun (lihat `middleware/app_setting.go`) — jadi row di sini seharusnya **selalu ketemu**. `code: 100` ("gagal ambil data brand") cuma bisa kejadian kalau brand-nya kehapus tepat di antara validasi middleware dan query ini (race yang sangat kecil kemungkinannya), tetap dihandle biar gak panic.
