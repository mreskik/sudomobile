# Auth - Login PIN

Login pakai PIN -- alternatif [Login OTP](LOGIN%20OTP.md), gak perlu nunggu kode OTP, tapi wajib
udah pernah [Create PIN](PIN%20CREATE.md) duluan (butuh login lewat OTP dulu sekali).

Wajib header `X-App-Setting`, sama kayak semua route lain di service ini.

```
POST /api/auth/login_pin
```

**Publik** (gak butuh token) -- ini justru salah satu cara buat DAPET token.

## Request

```json
{
  "phone_number": "62899888712345",
  "pin": "778899"
}
```

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "token": "68d2bb16cd5d615a88095725b57bab8f24b5931cb7ed6d5ea5c7f60ef9c25877",
    "member": {
      "id": 22,
      "code": "MOB0001",
      "name": "Test 4Step",
      "phone_number": "62899888712345"
    }
  }
}
```

Error yang mungkin balik (semua tetep HTTP `200`, `code: 100`):

| `message` | Kapan |
| --- | --- |
| `phone_number wajib diisi` / `pin wajib diisi` | Field kosong |
| `nomor belum terdaftar` | `phone_number` gak ketemu di `master_member` (atau `is_active = false`) |
| `pin belum pernah diset, silakan login pakai otp dulu` | Member-nya ada, tapi belum pernah manggil `pin/create` |
| `pin salah` | PIN-nya ada, tapi gak cocok sama hash yang kesimpen |

## Catatan

- Sukses login PIN = **session baru** (baris baru di `mobile_member_session`, umur 30 hari),
  sama pola kayak `register`/`login_otp` -- bukan reuse token lama.
- Pesan errornya sengaja dibedain 3 kasus (`belum terdaftar` vs `belum set PIN` vs `salah`) --
  biar app bisa nunjukin UI yang beda-beda (misal "belum terdaftar" arahin ke `register`,
  "belum set PIN" arahin ke `login_otp` dulu).

## Tervalidasi live (2026-08-20)

- PIN salah -> `"pin salah"`.
- PIN bener -> sukses, session token baru.
- Nomor yang gak ada di `master_member` -> `"nomor belum terdaftar"`.
