# Account - Update Photo

Ganti foto profil member yang lagi login.

```
POST /api/account/photo
Authorization: Bearer <token>
Content-Type: multipart/form-data
```

**Protected** — `member_id` dari session token, sama pola kayak [ME.md](ME.md).

## Request

Multipart form, field `file` (satu-satunya field).

- **Ukuran maksimal 2MB** — sama persis konvensi `maxImageSize` di modul upload `sudocore2` (`backend/modules/upload/upload_service.go`).
- **Ekstensi diizinkan**: `.jpg`, `.jpeg`, `.png`, `.webp`, `.gif`.

## Response

```json
{ "code": 0, "message": "success", "data": { "profile_photo_src": "/storage/uploads/images/0199...-....jpg" } }
```

`profile_photo_src` — path relatif, sama nilai yang abis ini muncul di [ME.md](ME.md#response) (`master_member.profile_photo_src` langsung keupdate). Diakses lewat base URL `sudomobile` sendiri (**bukan** base URL `sudocore2`/ERP — lihat "Catatan" soal storage terpisah).

Error yang mungkin balik (semua tetep HTTP `200`, `code: 100`):

| `message` | Kapan |
|---|---|
| `token tidak ditemukan` / `token tidak valid` | Session invalid (dari middleware, sebelum ke handler) |
| `batas ganti foto profil hari ini udah abis (maks 3x), coba lagi besok` | Udah 3x ganti foto **hari ini** (lihat "Catatan") |
| `file wajib diisi` | Field `file` gak dikirim |
| `ukuran file maksimal 2 MB` | File lebih dari 2MB |
| `tipe file tidak diizinkan` | Ekstensi bukan salah satu dari `.jpg`/`.jpeg`/`.png`/`.webp`/`.gif` |

## Catatan

- **Barrier maks 3x ganti per HARI KALENDER** (bukan rolling 24 jam) — dihitung dari `mobile_member_photo_change_log`, `COUNT(*) WHERE member_id = ? AND created_at >= CURRENT_DATE`. Reset otomatis pas tanggal berganti (predictable buat user: "besok reset", bukan "24 jam dari upload terakhir").
- **Storage SENDIRI di `sudomobile`** (2026-08-21), **bukan** numpang ke endpoint upload `sudocore2` yang udah ada (`/backend/data/upload/image`) — keputusan sadar biar `sudomobile` tetap self-contained, gak nambah dependency HTTP antar-service baru. Konsekuensinya: `profile_photo_src` di-resolve dari base URL `sudomobile`, **beda** dari field foto lain di sistem (logo cabang, dst) yang base URL-nya `sudocore2`. Konvensi ukuran/ekstensi file sengaja **disamain persis** modul upload `sudocore2` walau kode-nya duplikat (beda module Go, gak bisa saling import package internal antar service terpisah).
- File disimpen di `sudomobile/storage/uploads/images/`, nama file `<uuid v7>.<ext>`, disajikan lewat static route `GET /storage/*`.
- Upload foto baru **gak ngehapus file lama** dari disk — sama pola kayak modul lain di ekosistem ini (logo branch, item image) yang emang gak pernah hapus file fisik lama, biar sederhana.
- Update `master_member.profile_photo_src` + insert baris `mobile_member_photo_change_log` itu **1 transaksi atomic** — kalau salah satu gagal, dua-duanya di-rollback (file yang udah kesimpen ke disk **tidak** ikut di-rollback, cuma DB-nya — file lama nganggur ini sama kayak poin di atas, dianggap gak masalah).

## Tervalidasi

- **Query rate-limit** (2026-08-21) — tervalidasi lewat `psql`: insert 3 baris manual, `COUNT` balik `3` sesuai ekspektasi (baris ke-4 bakal ke-block di kode). `go build`/`go vet` bersih.

**⚠️ Belum tervalidasi lewat HTTP request beneran** (upload file asli, static serving-nya) — belum sempat dicoba lewat request HTTP (server dev butuh restart). Update bagian ini kalau udah dites.
