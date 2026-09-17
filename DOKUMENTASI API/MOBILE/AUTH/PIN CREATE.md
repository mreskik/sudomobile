# Auth - Create PIN

Bikin PIN (6 digit) buat member yang lagi login, **CUMA buat pertama kali**. PIN itu alternatif
OTP -- lebih cepet (gak perlu nunggu kode), tapi **wajib udah pernah login lewat OTP dulu
sekali** buat set PIN-nya. Setelah member punya PIN, dia bisa login pakai [Login PIN](LOGIN%20PIN.md)
tanpa OTP lagi.

**⚠️ Cuma buat sekali** (2026-08-21) -- kalau member udah punya PIN, ditolak, harus lewat
[Change PIN](PIN%20CHANGE.md). Sebelumnya endpoint ini berlaku upsert (manggil lagi = ganti PIN
lama diam-diam) -- itu behavior LAMA, udah gak berlaku.

Wajib header `X-App-Setting`, sama kayak semua route lain di service ini.

```
POST /api/auth/pin/create
Authorization: Bearer <token>
```

**Protected** -- wajib header `Authorization: Bearer <session_token>` (hasil `register`/
`login_otp`/`login_pin` sebelumnya). `member_id`-nya diambil dari session token itu, **BUKAN**
dari body request -- user cuma bisa bikin PIN akun sendiri.

## Request

```json
{ "pin": "778899" }
```

- `pin` -- wajib, **persis 6 digit angka**.

## Response

```json
{ "code": 0, "message": "pin berhasil disimpan", "data": null }
```

Kalau belum login / token invalid (ditolak di middleware, sebelum sempat ke handler ini):

```json
{ "code": 100, "message": "token tidak ditemukan", "data": null }
```
```json
{ "code": 100, "message": "token tidak valid", "data": null }
```

Kalau `pin` bukan 6 digit angka:

```json
{ "code": 100, "message": "pin harus 6 digit angka", "data": null }
```

Kalau member ini **udah punya PIN** (manggil `pin/create` lagi setelah pernah sukses sebelumnya):

```json
{ "code": 100, "message": "pin sudah pernah dibuat, gunakan ganti pin", "data": null }
```

## Catatan

- **1 member cuma boleh punya 1 PIN** -- `mobile_member_pin.member_id` UNIQUE. `pin/create`
  **cuma buat pertama kali** -- kalau udah ada baris PIN buat member ini, ditolak (lihat response
  di atas), gak di-update diam-diam. Ganti PIN yang udah ada = [Change PIN](PIN%20CHANGE.md).
- **PIN di-hash pakai bcrypt** (`golang.org/x/crypto/bcrypt`, cost default) -- **gak pernah**
  disimpen plaintext, gak ada cara buat "liat balik" PIN yang udah diset.
- **Lupa PIN (gak tau PIN lama)?** Pakai [Reset PIN](PIN%20RESET.md) -- verifikasi ulang lewat
  OTP, bukan verifikasi PIN lama kayak [Change PIN](PIN%20CHANGE.md).

## Tervalidasi live (2026-08-20)

- Tanpa `Authorization` -> `"token tidak ditemukan"`.
- Dengan token ngasal -> `"token tidak valid"`.
- Token valid, PIN gak 6 digit (`"123"`) -> `"pin harus 6 digit angka"`.
- Token valid, PIN 6 digit -> sukses, baris baru di `mobile_member_pin` (`updated_at` masih
  `null` karena ini INSERT pertama, bukan update).

**⚠️ Belum tervalidasi live** (2026-08-21): behavior baru nolak kalau udah punya PIN. Baru lolos
`go build`/`go vet` (compile-time doang) -- belum dicoba lewat request beneran ke server jalan.
Update bagian ini kalau udah dites.
