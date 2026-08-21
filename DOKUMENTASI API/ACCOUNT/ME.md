# Account - Me

Profil akun customer yang lagi login. **Read-only** — belum ada endpoint edit profil di sini (belum ada fitur "lengkapi profil"/"edit profil").

Wajib header `X-App-Setting`, sama kayak semua route lain di service ini.

```
GET /api/account/me
Authorization: Bearer <token>
```

**Protected** — wajib header `Authorization: Bearer <session_token>` (hasil `register`/`login_otp`/`login_pin`/`pin/reset`). `member_id`-nya diambil dari session token itu, **BUKAN** dari param — user cuma bisa liat data akun sendiri.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 22,
    "code": "MOB0001",
    "name": "Test 4Step",
    "phone_number": "62899888712345",
    "email": null,
    "gender": null,
    "profile_photo_src": null,
    "member_since": "2026-08-11T09:51:06+07:00",
    "has_pin": true
  }
}
```

| Field | Tipe | Sumber | Keterangan |
|---|---|---|---|
| `id` | number | `master_member.id` | |
| `code` | string | `master_member.code` | Kode member (`MOB0001`, dst) |
| `name` | string | `master_member.name` | |
| `phone_number` | string | `master_member.phone_number` | |
| `email` | string, nullable | `master_member.email` | `register` gak minta field ini — `null` sampai ada fitur lengkapi profil |
| `gender` | string, nullable | `master_member.gender` | `"male"`/`"female"`, `null` kalau belum diisi |
| `profile_photo_src` | string, nullable | `master_member.profile_photo_src` | Path foto profil, `null` kalau belum upload |
| `member_since` | datetime | alias `master_member.created_at` | Nama field lebih deskriptif dari `created_at` yang kedengeran teknis |
| `has_pin` | bool | derived, `EXISTS` di `mobile_member_pin` | Dipakai app buat mutusin nunjukin prompt "aktifkan login PIN" atau nyembunyiin opsi "Login pakai PIN" |

Kalau belum login / token invalid (ditolak di middleware, sebelum sempat ke handler ini):

```json
{ "code": 100, "message": "token tidak ditemukan", "data": null }
```
```json
{ "code": 100, "message": "token tidak valid", "data": null }
```

## Catatan

- **Prefix `account`, bukan `user`** (2026-08-21) — sengaja dibedain dari istilah `user` yang di ekosistem ini (`sudocore2`) udah dipakai buat akun **staff/admin ERP** (`master_user`), beda konsep total dari customer (`master_member`). `account` lebih natural juga buat app consumer ("My Account", dst).
- **Field yang sengaja gak diikutin**: `member_type_id`/nama tier (selalu kosong buat member yang daftar sendiri lewat mobile — belum ada mekanisme assign tier lewat mobile, lihat [MASTER MEMBER TIER SETTING.md](../../../sudocore2/DOKUMENTASI%20API/MASTER/MASTER%20MEMBER%20TIER%20SETTING.md) di `sudocore2` buat evaluasi tier via background job), `is_active` (redundan — kalau session-nya valid berarti pasti aktif, semua endpoint login udah filter `is_active = true`), `contact_name`/`created_by`/`updated_by`/`updated_at` (gak relevan buat customer).
- `gender`/`profile_photo_src` kolomnya ada di `master_member` (ERP, `sudocore2`, ditambah 2026-08-21) — bukan tabel `mobile_*` terpisah, karena `master_member` udah punya mirror sync ke POS (`mr_member`) yang bisa dipakai ulang kalau nanti POS butuh data ini juga.

## Tervalidasi

- **Query** (2026-08-21) — tervalidasi langsung lewat `psql` ke data member asli, semua kolom ke-return sesuai ekspektasi (termasuk `gender`/`profile_photo_src` kosong buat member yang belum ngisi, `has_pin: false` buat yang belum pernah `pin/create`). `go build`/`go vet` bersih.

**⚠️ Belum tervalidasi lewat HTTP request** — belum sempat dicoba lewat request HTTP beneran (server dev butuh restart). Update bagian ini kalau udah dites.
