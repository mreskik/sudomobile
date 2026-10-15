# Account - Me

Profil akun customer yang lagi login. Endpoint ini **read-only** — buat edit `name`/`email`/`gender`/`birth_date`, lihat [UPDATE ME.md](UPDATE%20ME.md) (endpoint terpisah, `PUT` bukan `GET`). `profile_photo_src` punya endpoint sendiri (beda alur, upload file), lihat [UPDATE PHOTO.md](UPDATE%20PHOTO.md).

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
    "birth_date": null,
    "description": null,
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
| `email` | string, nullable | `master_member.email` | `register` gak minta field ini, jadi awalnya `null` — bisa diisi **SEKALI** lewat [UPDATE ME.md](UPDATE%20ME.md) (2026-10-13), sesudah itu **gak bisa diubah/dikosongkan lagi**. Cuma validasi format, bukan verifikasi (gak ada link konfirmasi) |
| `gender` | string, nullable | `master_member.gender` | `"male"`/`"female"`, `null` kalau belum diisi. Bisa diisi **SEKALI** lewat [UPDATE ME.md](UPDATE%20ME.md), sesudah itu gak bisa diubah lagi |
| `birth_date` | string, nullable | `master_member.birth_date` | Format `"YYYY-MM-DD"` (bukan RFC3339), di-cast via `to_char()` di query. `null` kalau belum diisi. Bisa diisi **SEKALI** lewat [UPDATE ME.md](UPDATE%20ME.md), sesudah itu gak bisa diubah lagi |
| `description` | string, nullable | `master_member.description` | Catatan bebas, `null` kalau belum diisi. **BEDA** dari `email`/`gender`/`birth_date` — bebas diedit berkali-kali (termasuk dikosongkan lagi) lewat [UPDATE ME.md](UPDATE%20ME.md), gak kena aturan "sekali isi lalu terkunci" |
| `profile_photo_src` | string, nullable | `master_member.profile_photo_src` | Path foto profil, `null` kalau belum upload. Cara ganti: [UPDATE PHOTO.md](UPDATE%20PHOTO.md) |
| `member_since` | datetime | alias `master_member.created_at` | Nama field lebih deskriptif dari `created_at` yang kedengeran teknis |
| `has_pin` | bool | derived, `EXISTS` di `mobile_member_pin` | Dipakai app buat mutusin nunjukin prompt "aktifkan login PIN" atau nyembunyiin opsi "Login pakai PIN" |

Mau info tier/spending/progress evaluasi? Itu ada di endpoint terpisah, [TIER AND SPENDING INFORMATION.md](TIER%20AND%20SPENDING%20INFORMATION.md) — sengaja **gak** digabung di sini (lihat "Catatan").

Kalau belum login / token invalid (ditolak di middleware, sebelum sempat ke handler ini):

```json
{ "code": 100, "message": "token tidak ditemukan", "data": null }
```
```json
{ "code": 100, "message": "token tidak valid", "data": null }
```

## Catatan

- **Prefix `account`, bukan `user`** (2026-08-21) — sengaja dibedain dari istilah `user` yang di ekosistem ini (`sudocore2`) udah dipakai buat akun **staff/admin ERP** (`master_user`), beda konsep total dari customer (`master_member`). `account` lebih natural juga buat app consumer ("My Account", dst).
- **Field yang sengaja gak diikutin**: `member_type_id` (selalu kosong buat member yang daftar sendiri lewat mobile), `is_active` (redundan — kalau session-nya valid berarti pasti aktif, semua endpoint login udah filter `is_active = true`), `contact_name`/`created_by`/`updated_by`/`updated_at` (gak relevan buat customer).
- `gender`/`profile_photo_src` kolomnya ada di `master_member` (ERP, `sudocore2`, ditambah 2026-08-21) — bukan tabel `mobile_*` terpisah, karena `master_member` udah punya mirror sync ke POS (`mr_member`) yang bisa dipakai ulang kalau nanti POS butuh data ini juga.
- `birth_date` — kolom yang **sudah ada** di `master_member` dari sebelumnya (migration `255_alter_table_master_member_add_birth_date.sql` sudocore2, dipakai bareng modul member ERP), baru dipakai di sudomobile (2026-10-13). Di-cast ke `"YYYY-MM-DD"` via `to_char()` langsung di query `fetchMe()` — bukan custom Go type kayak `DateOnly` di sudocore2, karena field lain di response `/me` juga polos `*string`/`time.Time` tanpa custom type. Bisa diisi/diedit lewat `UpdateMe`, lihat [UPDATE ME.md](UPDATE%20ME.md).
- **`tier` sempet ada di sini, sekarang dipindah** (2026-08-21) ke [TIER AND SPENDING INFORMATION.md](TIER%20AND%20SPENDING%20INFORMATION.md) — digabung bareng `spending_total`/`next_evaluation` karena ketiganya satu concern yang sama (basisnya sama-sama dari `master_member_tier_setting`), sementara `/me` dijaga tetap murni data profil statis (jarang berubah) — beda karakteristik refresh dari tier/spending yang lebih sering di-refresh.
- `description` (2026-10-15) — kolom yang **sudah ada** di `master_member` dari sebelumnya (dipakai modul Member ERP sudocore2, admin bisa isi catatan bebas soal member), baru diekspos ke sudomobile. Beda dari `email`/`gender`/`birth_date`: **gak kena aturan "sekali isi"** — field ini murni catatan bebas (bukan data identitas permanen), jadi bisa diedit berkali-kali termasuk dikosongkan lagi lewat `UpdateMe`.

## Tervalidasi

- **Query** (2026-08-21) — tervalidasi langsung lewat `psql` ke data member asli, semua kolom ke-return sesuai ekspektasi (termasuk `gender`/`profile_photo_src` kosong buat member yang belum ngisi, `has_pin: false` buat yang belum pernah `pin/create`). `go build`/`go vet` bersih.
- **`description`** (2026-10-15) — query `fetchMe()` yang sudah ditambah kolom ini dites langsung lewat `psql` (member id 23, isinya `"hehe"` dari data existing) — muncul sesuai ekspektasi. `go build`/`go vet` bersih.

**⚠️ Belum tervalidasi lewat HTTP request** — belum sempat dicoba lewat request HTTP beneran (server dev butuh restart). Update bagian ini kalau udah dites.
