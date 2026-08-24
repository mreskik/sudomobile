# Auth - Logout

```
POST /api/auth/logout
```

Hapus session yang lagi dipakai (token dari header `Authorization` request ini). **PROTECTED**
(wajib `Authorization: Bearer <token>`, sama kayak semua route protected lain di service ini).

Wajib header `X-App-Setting`, sama kayak semua route lain di service ini.

## Request

Gak ada body. Cukup header:

```
Authorization: Bearer <token>
```

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

Gak ada error case spesifik -- selama `Authorization` header lolos `middleware.Auth` (token
valid & belum expired), logout selalu sukses. Kalau token udah gak valid/expired, request-nya
malah ketolak duluan di `middleware.Auth` sebelum sampai ke handler ini (`"token tidak
ditemukan"` / `"token tidak valid"`, sama pesan kayak protected route lain).

## Catatan

- **Scope-nya SATU device doang** -- yang dihapus cuma baris `mobile_member_session` yang
  `token`-nya persis sama dengan yang dipakai buat request ini. Device/session lain milik
  member yang sama (kalau login di lebih dari 1 device) **tetep login**, gak ikut ke-revoke.
  Ini konsisten sama filosofi yang udah dipakai [Reset PIN](PIN%20RESET.md) (reset PIN juga
  sengaja gak nge-revoke session lain).
- **Hard delete** (`DELETE FROM mobile_member_session WHERE token = ?`), bukan soft-delete
  (kolom `revoked_at` dst) -- konsisten sama gaya modul lain di codebase ini yang belum ada
  kebutuhan audit trail buat tabel session. Kalau nanti butuh histori login/logout, itu
  perubahan skema terpisah.
- Handler ini **parse ulang** `Authorization` header (bukan cuma pakai `member_id` yang udah
  disimpen `middleware.Auth` ke locals) -- karena yang mau dihapus itu barisnya berdasarkan
  `token`, bukan `member_id` (member yang sama bisa punya banyak baris session kalau
  multi-device, gak boleh salah hapus punya device lain).
- Setelah logout, token itu langsung gak valid lagi buat semua endpoint protected lain (`GET
  /account/me` dst bakal balik `"token tidak valid"` kalau masih dipakai).

## Tervalidasi live

**⚠️ Belum tervalidasi live** -- endpoint ini baru lolos `go build`/`go vet` (compile-time
doang), belum dicoba lewat request beneran ke server jalan. Update bagian ini kalau udah dites.
