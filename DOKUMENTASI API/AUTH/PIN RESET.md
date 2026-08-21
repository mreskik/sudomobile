# Auth - Reset PIN

Buat member yang **lupa PIN lama** (2026-08-21) -- gak bisa lewat [Change PIN](PIN%20CHANGE.md)
karena itu wajib tau PIN lama. Reset PIN sebagai gantinya minta bukti kepemilikan nomor lewat
**OTP fresh**, bukan verifikasi PIN lama.

**Publik** (gak butuh token) -- alurnya: [Request OTP](REQUEST%20OTP.md) dengan
`type: "reset_pin"` dulu, terus panggil endpoint ini bareng kodenya + PIN baru.

Wajib header `X-App-Setting`, sama kayak semua route lain di service ini.

```
POST /api/auth/pin/reset
```

## Request

```json
{
  "phone_number": "62899888712345",
  "otp": "1234",
  "new_pin": "998877"
}
```

- `phone_number` -- wajib, nomor yang sama pas minta OTP.
- `otp` -- wajib, kode OTP dari [Request OTP](REQUEST%20OTP.md) `type: "reset_pin"`.
- `new_pin` -- wajib, **persis 6 digit angka**.

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

Sukses reset PIN **langsung dapet session token baru sekalian** (sama pola kayak
[Register](REGISTER.md)/[Login OTP](LOGIN%20OTP.md)) -- gak perlu manggil
[Login PIN](LOGIN%20PIN.md) terpisah lagi setelah ini.

Error yang mungkin balik (semua tetep HTTP `200`, `code: 100`):

| `message` | Kapan |
| --- | --- |
| `phone_number wajib diisi` / `otp wajib diisi` | Field kosong |
| `pin baru harus 6 digit angka` | `new_pin` bukan persis 6 digit angka |
| `nomor belum terdaftar` | `phone_number` gak ketemu di `master_member` (atau `is_active = false`) |
| `otp tidak ditemukan atau sudah kedaluwarsa` | Belum pernah minta OTP `type=reset_pin` buat nomor ini, atau udah expired (5 menit) |
| `otp salah` | Ada OTP `type=reset_pin` yang masih valid, tapi kodenya gak cocok |

## Catatan

- **PIN langsung di-overwrite (upsert)**, gak peduli member ini udah punya PIN sebelumnya atau
  belum -- beda dari [Create PIN](PIN%20CREATE.md) yang nolak kalau udah ada. Kalau memang belum
  pernah punya PIN sama sekali, pakai endpoint ini juga jadi cara pintas set PIN pertama sambil
  bukti nomor lewat OTP (gak perlu login OTP dulu terus manggil Create PIN terpisah).
- OTP-nya **wajib `type: "reset_pin"`** -- OTP yang diminta buat `register`/`login` gak bisa
  dipakai di sini (isolasi per-`type`, sama pola semua endpoint OTP lain, lihat
  [Request OTP](REQUEST%20OTP.md)).
- Sukses reset PIN **tidak** nge-revoke session lain yang lagi aktif (`mobile_member_session`
  gak disentuh selain nambah baris baru) -- device lain yang masih login tetep login.
- Gak ada limit percobaan `otp` salah (belum ada rate-limit/lockout) -- kalau nanti mau
  ditambah, ini titik yang relevan (sama kayak semua endpoint verifikasi OTP lain).

## Tervalidasi live

**⚠️ Belum tervalidasi live** (2026-08-21) -- endpoint ini baru lolos `go build`/`go vet`
(compile-time doang), belum dicoba lewat request beneran ke server jalan. Update bagian ini
kalau udah dites.
