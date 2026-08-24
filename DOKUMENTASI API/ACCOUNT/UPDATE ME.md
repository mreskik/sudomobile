# Account - Update Me

Edit profil member yang lagi login. **Baru `name`+`gender` doang buat sekarang** (2026-08-21) — field lain (`email`, `profile_photo_src`, dst) belum ada cara editnya lewat sini.

```
PUT /api/account/me
Authorization: Bearer <token>
Content-Type: application/json
```

**Protected** — `member_id` dari session token, sama pola kayak [ME.md](ME.md).

## Request

```json
{ "name": "Budi Santoso", "gender": "male" }
```

- `name` — **wajib**, gak boleh kosong (setelah di-trim spasi).
- `gender` — **opsional**: kirim `"male"`/`"female"` buat set, kirim **string kosong `""`** buat **kosongin lagi** (`null`). Field ini **selalu diproses ulang tiap request** — bukan partial-update ("gak dikirim = gak berubah"), jadi kalau mau `gender`-nya tetap kayak sebelumnya, tetap wajib disertain nilainya di body.

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
    "email": "reski@example.com",
    "gender": "male",
    "profile_photo_src": null,
    "member_since": "2026-07-16T14:21:26+07:00",
    "has_pin": false
  }
}
```

Error yang mungkin balik (semua tetep HTTP `200`, `code: 100`):

| `message` | Kapan |
|---|---|
| `token tidak ditemukan` / `token tidak valid` | Session invalid (dari middleware, sebelum ke handler) |
| `body request tidak valid` | JSON body gak valid |
| `name wajib diisi` | `name` kosong (atau cuma spasi) |
| `gender wajib "male", "female", atau dikosongin` | `gender` diisi selain 3 kemungkinan itu |

## Catatan

- Cuma `name`/`gender` yang bisa diubah lewat endpoint ini. `email` belum ada cara editnya sama sekali (dari mana pun) — masih `null` sampai ada fitur "lengkapi profil" yang lebih lengkap. `profile_photo_src` punya endpoint sendiri, lihat [UPDATE PHOTO.md](UPDATE%20PHOTO.md).
- Gak ada validasi panjang/format `name` di luar "gak boleh kosong" — bebas.
- Gak ada limit berapa kali boleh ganti `name`/`gender` per hari (beda dari [UPDATE PHOTO.md](UPDATE%20PHOTO.md) yang dibatasi 3x/hari) — data teks ini dianggap gak butuh proteksi seketat foto (lebih murah buat di-review/gak disalahgunakan dibanding upload file).

## Tervalidasi

- **Query** (2026-08-21) — tervalidasi langsung lewat `psql` ke data member asli (`id=14`): update `name`+`gender: "male"` berhasil, terus kirim `gender: ""` (via `NULL` manual) balik ngosongin lagi ke `null` sesuai ekspektasi. Data dibalikin ke kondisi semula setelah tes. `go build`/`go vet` bersih.

**⚠️ Belum tervalidasi lewat HTTP request** — belum sempat dicoba lewat request HTTP beneran (server dev butuh restart). Update bagian ini kalau udah dites.
