# Auth - Check Number

```
POST /api/auth/check_number
```

Cek 1 nomor HP udah kedaftar di `master_member` apa belum. Dipanggil app **sebelum** minta
OTP -- biar app bisa nentuin mau nunjukin layar "login" atau "daftar" duluan. Nomor gak
kedaftar **bukan error**, tetep `code: 0`, jawabannya di `is_registered`.

Wajib header `X-App-Setting` (lihat `../README` root `sudomobile` -- ciphertext AES-256-GCM,
isi `db_code`+`company_id`), sama kayak semua route lain di service ini.

## Request

```json
{
  "phone_number": "62812345678901"
}
```

- `phone_number` -- wajib diisi. **Harus udah dalam format final** (kode negara TANPA tanda
  `+`, contoh `62812345678901`)

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "phone_number": "62812345678901",
    "is_registered": true
  }
}
```

- `phone_number` di response **persis apa adanya dari input**, gak diubah sama sekali.
- `is_registered` -- `true` kalau ketemu baris `master_member` dengan `phone_number` itu DAN
  `is_active = true`. Member yang di-nonaktifin dianggap gak kedaftar. Format yang gak match
  convention (misal masih `08xx`) otomatis gak ketemu -- balik `false`, bukan error.

Kalau `phone_number` kosong:

```json
{ "code": 100, "message": "phone_number wajib diisi", "data": null }
```

## Catatan

- **Convention nomor HP di sistem ini: kode negara TANPA plus** (`62812xxxxxxx`), bukan
  `+62812xxxxxxx` maupun `0812xxxxxxx`. Ini juga format yang sekarang dipakai di kolom
  `master_member.phone_number` (dinormalisasi lewat migration `099` di `sudocore2`, sebelumnya
  campur-campur format lokal + ada data sampah/testing yang bukan nomor HP sama sekali).
- **SENGAJA gak ada normalisasi di endpoint ini** -- `phone_number` di-_exact match_ apa
  adanya ke DB, gak diutak-atik backend sama sekali. **Frontend yang wajib** selalu kirim
  format `62xxx` yang bener. Kenapa gitu (bukan backend yang normalisasi kayak sempat
  diimplementasi sebelumnya): biar cuma ada 1 sisi yang nentuin/nge-generate format nomor
  (frontend), gak ada 2 sisi (frontend & backend) yang sama-sama nyoba "nebak"/ngubah format
  sendiri-sendiri, yang ujungnya malah rawan bentrok kalau logic normalisasinya beda tafsir.
- **Query-nya exact match**: `SELECT COUNT(*) FROM master_member WHERE phone_number = ? AND
is_active = true`. Gak ada fallback ke format lain.

## Tervalidasi live (2026-08-20)

- Nomor kedaftar (id 1), dikirim persis format `62812345678901` -> `is_registered: true`.
- Nomor sama, dikirim format lokal `0812345678901` (bukan format convention) -> `is_registered:
false` (disengaja, bukan bug -- backend gak nebak-nebak format).
- `phone_number` kosong -> `code: 100`, `"phone_number wajib diisi"`.
- `phone_number` isinya sampah (`"asd"`) -> **gak ditolak**, diterima & diproses biasa,
  hasilnya `is_registered: false` (gak ketemu, bukan error -- endpoint ini gak validasi format,
  cuma exact-match ke DB).
- Request tanpa header `X-App-Setting` -> tetep ketolak di middleware duluan (`"X-App-Setting
wajib diisi"`), gak sempat ke handler ini.
- Semua respons di atas balik HTTP `200` -- sukses/gagal ditentuin dari `code` di body, bukan
  HTTP status (sama convention `sudocore2`/`APIANDORDER`).
