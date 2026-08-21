# Auth - Login OTP

```
POST /api/auth/login_otp
```

Login pakai OTP buat nomor yang **UDAH** kedaftar -- kebalikan `register` (gak ada `name`,
gak insert `master_member` baru, cuma cari member yang udah ada & terbitin session token
baru). Dipanggil setelah `request_otp` dengan `type=login`.

Wajib header `X-App-Setting`, sama kayak semua route lain di service ini.

## Request

```json
{
  "phone_number": "62899888712345",
  "otp": "0613"
}
```

- `phone_number` -- wajib, format `62xxx` (gak dinormalisasi, sama kayak endpoint auth lain).
- `otp` -- wajib, kode 4 digit hasil `request_otp` **`type=login`** buat nomor yang sama.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "token": "dbdee8e130cb9541c10b83dbc552c95e0393c94e91a741bbdeeaafedbb46c22e",
    "member": {
      "id": 22,
      "code": "MOB0001",
      "name": "Test 4Step",
      "phone_number": "62899888712345"
    }
  }
}
```

- `token` -- session baru, umur 30 hari, sama pola kayak `register` (baris baru di
  `mobile_member_session`, bukan reuse token lama -- tiap login = session baru).
- `member` -- data member yang **udah ada**, gak berubah field-nya di sini (login doang, bukan
  update profile).

Error yang mungkin balik (semua tetep HTTP `200`, `code: 100`):

| `message` | Kapan |
| --- | --- |
| `phone_number wajib diisi` / `otp wajib diisi` | Field kosong |
| `nomor belum terdaftar` | `phone_number` gak ketemu di `master_member` (atau `is_active = false`) -- suruh `register` dulu |
| `otp tidak ditemukan atau sudah kedaluwarsa` | Gak ada baris `mobile_member_otp` `type='login'` buat nomor ini yang masih valid |
| `otp salah` | Ada OTP `type=login` valid, tapi kode-nya gak cocok |

## Catatan

- **Pola sama persis kayak `register`** (transaksi `BeginTx`/`Commit`/`Rollback`, mark
  `verified_at`, insert session) -- bedanya cuma gak ada insert `master_member` (member-nya
  udah ada), dan gak ada `name` di request.
- OTP-nya WAJIB `type='login'` -- lihat `REQUEST OTP.md` soal isolasi `type`.

## Tervalidasi live (2026-08-20)

- Request OTP `type=login`, dipake di sini -- sukses, session token baru kebuat, data member
  balik apa adanya.
- Nomor yang gak kedaftar -> `"nomor belum terdaftar"`.
