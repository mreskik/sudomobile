# Account - Update Me

Edit profil member yang lagi login. **`name`+`email`+`gender`+`birth_date`** (2026-10-13: `gender`/`birth_date` sempet dipisah jadi endpoint sendiri-sendiri, digabung lagi ke sini; `email` baru ditambah) — field lain (`profile_photo_src`) punya endpoint sendiri karena upload file, lihat [UPDATE PHOTO.md](UPDATE%20PHOTO.md).

```
PUT /api/account/me
Authorization: Bearer <token>
Content-Type: application/json
```

**Protected** — `member_id` dari session token, sama pola kayak [ME.md](ME.md).

## Request

```json
{ "name": "Budi Santoso", "email": "budi@example.com", "gender": "male", "birth_date": "1995-03-20" }
```

- `name` — **wajib**, gak boleh kosong (setelah di-trim spasi). **Selalu bisa diubah berkali-kali** — gak kena aturan "sekali isi" di bawah.
- `email` — **opsional**: kirim alamat email valid buat set. Cuma divalidasi **format**-nya (pakai `net/mail` standar Go) — **TIDAK** ada validasi uniqueness lintas member (email boleh sama antar member, `phone_number` tetap identitas utama/unik).
- `gender` — **opsional**: kirim `"male"`/`"female"` buat set.
- `birth_date` — **opsional**: format wajib **`"YYYY-MM-DD"`** (bukan RFC3339/ISO datetime penuh) kalau diisi — kirim `"1995-03-20"`, **BUKAN** `"1995-03-20T00:00:00Z"`. Format salah ditolak dengan pesan jelas sebelum sampai ke query database.

**⚠️ `email`/`gender`/`birth_date` cuma bisa diisi SEKALI** (2026-10-13) — begitu salah satu kolom ini **sudah terisi** di database (bukan `null`), request **apa pun** yang menyertakan field itu (nilai apa pun, **termasuk nilai yang sama persis** kayak yang udah tersimpan) akan **ditolak dengan error jelas**, sebelum sampai nyentuh database. Field yang masih `null` tetap bisa diisi normal.

- `""` (string kosong) atau **gak sertain field-nya sama sekali** di body = "gak ada perubahan yang diminta" buat field itu — **selalu aman dikirim**, baik kolomnya masih `null` maupun udah terisi, **gak pernah** kena validasi "sudah terisi" di atas.

## Response

Balikin data profil **terbaru** (bentuk sama kayak [ME.md](ME.md)) — biar app langsung update tampilan tanpa perlu request `/me` terpisah lagi.

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 14,
    "code": "CUS001",
    "name": "Budi Santoso",
    "phone_number": "6285706475945",
    "email": "budi@example.com",
    "gender": "male",
    "birth_date": "1995-03-20",
    "profile_photo_src": null,
    "member_since": "2026-07-16T14:21:26+07:00",
    "has_pin": false
  }
}
```

Error yang mungkin balik (semua tetep HTTP `200`, `code: 100`):

| `message` | Kapan |
|---|---|
| `token not found` / `invalid token` | Session invalid (dari middleware, sebelum ke handler) |
| `invalid request body` | JSON body gak valid |
| `name is required` | `name` kosong (atau cuma spasi) |
| `invalid email format` | `email` diisi tapi formatnya bukan alamat email valid |
| `gender must be "male", "female", or empty` | `gender` diisi selain 3 kemungkinan itu |
| `invalid date format, must be YYYY-MM-DD` | `birth_date` diisi tapi formatnya salah (bukan `""` dan bukan `YYYY-MM-DD` valid) |
| `failed to fetch current profile data` | Gagal query cek state existing (sebelum validasi "sudah terisi") |
| `email has already been set and cannot be changed` | `email` di database **udah terisi**, tapi request masih menyertakan `email` (nilai apa pun) |
| `gender has already been set and cannot be changed` | `gender` di database **udah terisi**, tapi request masih menyertakan `gender` |
| `birth_date has already been set and cannot be changed` | `birth_date` di database **udah terisi**, tapi request masih menyertakan `birth_date` |

## Catatan

- `name`/`email`/`gender`/`birth_date` semuanya bisa diisi lewat endpoint ini, tapi **`email`/`gender`/`birth_date` masing-masing cuma sekali** — lihat aturan "sekali isi" di atas. `profile_photo_src` punya endpoint sendiri, lihat [UPDATE PHOTO.md](UPDATE%20PHOTO.md) (beda alur karena upload file, bukan form JSON biasa).
- **Implementasi**: sebelum `UPDATE`, handler query dulu state `email`/`gender`/`birth_date` existing (`SELECT ... FROM master_member WHERE id = ?`). Kalau field yang mau diisi ternyata udah `NOT NULL` di database, request ditolak LANGSUNG (gak ada `UPDATE` yang jalan sama sekali) — bukan partial update sebagian field berhasil sebagian ditolak, satu error bikin seluruh request gagal.
- Query `UPDATE` sendiri pakai `COALESCE(?, kolom)` per field — `""`/gak dikirim di-translate jadi `nil` (`*string`) di Go, ketemu `NULL` di SQL, `COALESCE` otomatis skip (pertahankan nilai lama). `name` TIDAK pakai `COALESCE` karena wajib selalu dikirim.
- **`email` sebelumnya gak ada cara editnya sama sekali** (dari mana pun) — selalu `null` sejak `register` (yang gak minta field ini). Mulai 2026-10-13 bisa diisi **sekali** lewat sini.
- Kolom `master_member.birth_date` (`DATE`, nullable) sudah ada dari sebelumnya (migration `255_alter_table_master_member_add_birth_date.sql` di sudocore2) — dipakai bareng modul member ERP (`backend/modules/master/member`), bukan kolom baru khusus sudomobile.
- **Tidak ada validasi umur/rentang tanggal** buat `birth_date` (misal menolak tanggal di masa depan, atau umur tidak realistis) — endpoint ini cuma validasi *format*, bukan *kewajaran nilai*. Sama halnya `email` — cuma validasi format, bukan verifikasi (tidak ada kirim link konfirmasi, dst).
- **Tidak ada jalur "koreksi kesalahan input"** — kalau member salah ketik `gender`/`birth_date`/`email` saat pengisian pertama, **tidak ada cara memperbaikinya lewat endpoint ini** (butuh intervensi manual, misal lewat admin ERP, kalau nanti dibutuhkan).
- Gak ada validasi panjang/format `name` di luar "gak boleh kosong" — bebas, dan `name` selalu bisa diubah berkali-kali tanpa batas.
- Gak ada limit berapa kali boleh ganti `name` per hari (beda dari [UPDATE PHOTO.md](UPDATE%20PHOTO.md) yang dibatasi 3x/hari).

## Tervalidasi

- `go build`/`go vet` bersih.

**⚠️ Belum tervalidasi lewat HTTP request / query live ke database** — belum sempat dicoba lewat request HTTP beneran. Update bagian ini kalau udah dites.
