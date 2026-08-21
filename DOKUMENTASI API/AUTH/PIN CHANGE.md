# Auth - Change PIN

Ganti PIN yang **udah ada** buat member yang lagi login (2026-08-21). Beda dari
[Create PIN](PIN%20CREATE.md): endpoint itu cuma buat pertama kali, ini buat member yang udah
punya PIN dan **inget PIN lamanya**. Kalau **lupa** PIN lama, pakai [Reset PIN](PIN%20RESET.md).

Wajib header `X-App-Setting`, sama kayak semua route lain di service ini.

```
POST /api/auth/pin/change
Authorization: Bearer <token>
```

**Protected** -- sama kayak `pin/create`, `member_id` dari session token, **BUKAN** dari body
request. Wajib ngirim PIN lama buat verifikasi -- gak cukup modal token session doang (token
bisa aja ke-compromise; PIN lama jadi lapisan verifikasi tambahan sebelum boleh ganti
credential).

## Request

```json
{ "old_pin": "778899", "new_pin": "112233" }
```

- `old_pin` -- wajib, PIN yang lagi aktif sekarang.
- `new_pin` -- wajib, **persis 6 digit angka**, PIN pengganti.

## Response

```json
{ "code": 0, "message": "pin berhasil diganti", "data": null }
```

Kalau belum login / token invalid (ditolak di middleware, sebelum sempat ke handler ini):

```json
{ "code": 100, "message": "token tidak ditemukan", "data": null }
```
```json
{ "code": 100, "message": "token tidak valid", "data": null }
```

Kalau `new_pin` bukan 6 digit angka:

```json
{ "code": 100, "message": "pin baru harus 6 digit angka", "data": null }
```

Kalau member ini **belum pernah** bikin PIN (harusnya pakai [Create PIN](PIN%20CREATE.md), bukan
`pin/change`):

```json
{ "code": 100, "message": "pin belum pernah dibuat, gunakan buat pin", "data": null }
```

Kalau `old_pin` **salah** (gak cocok sama hash yang kesimpen):

```json
{ "code": 100, "message": "pin lama salah", "data": null }
```

## Catatan

- Gak ada limit percobaan `old_pin` salah (belum ada rate-limit/lockout) -- kalau nanti mau
  ditambah, ini titik yang relevan.
- `new_pin` boleh sama persis kayak `old_pin` -- gak divalidasi harus beda.
- Sukses ganti PIN **tidak** nge-revoke session lain yang lagi aktif (`mobile_member_session`
  gak disentuh) -- device lain yang masih login tetep login, cuma PIN-nya yang berubah.
- Endpoint ini **wajib tau PIN lama** -- gak nolongin kalau lupa PIN. Kalau lupa, pakai
  [Reset PIN](PIN%20RESET.md) (verifikasi ulang lewat OTP, bukan verifikasi PIN lama).

## Tervalidasi live

**⚠️ Belum tervalidasi live** (2026-08-21) -- endpoint ini baru lolos `go build`/`go vet`
(compile-time doang), belum dicoba lewat request beneran ke server jalan. Update bagian ini
kalau udah dites.
