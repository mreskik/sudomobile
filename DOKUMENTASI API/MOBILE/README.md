# DOKUMENTASI API — sudomobile

Panduan umum sebelum baca dokumentasi per-endpoint (`AUTH/`, `ACCOUNT/`, dst) — 2 lapisan otorisasi yang berlaku di **semua** endpoint di service ini, header wajib apa aja, dan format response standar.

## 1. Header wajib — `X-App-Setting` (SEMUA route, termasuk publik)

Setiap request ke `sudomobile` — **termasuk endpoint publik** yang belum butuh login (`check_number`, `request_otp`, dst) — wajib ngirim header ini. Ini bukan identitas customer, ini konfigurasi app per-instalasi (semacam API key).

```
X-App-Setting: <ciphertext base64>
```

Isinya **ciphertext**, bukan plaintext — AES-256-GCM, di-encode `base64(nonce + ciphertext)`. Plaintext-nya (sebelum dienkripsi) format query-string:

```
db_code=tesmisal&company_id=15&brand_id=6
```

| Field | Wajib | Keterangan |
|---|---|---|
| `db_code` | Ya (non-empty) | Belum divalidasi ke mana-mana saat ini — sekadar ditampung, belum diputusin buat apa persisnya |
| `company_id` | Ya (angka) | Divalidasi eksistensinya ke `master_company` |
| `brand_id` | Ya (angka) | 2026-08-24 — divalidasi eksistensinya ke `master_brand`. Dipakai buat scoping data yang brand-specific (misal `master_image_mb_cust`, lihat `MASTER IMAGE MOBILE CUSTOMER.md` di sudocore2), baca lewat `middleware.BrandID(c)` |

**Cara bikin ciphertext-nya**: aplikasi client (mobile app) enkripsi plaintext di atas pakai key `APP_SETTING_KEY` (AES-256-GCM) sebelum kirim request. Key ini **statis, sama buat semua instalasi app** saat ini — nyegah request iseng (curl manual, network sniffing kasual), **bukan** proteksi kelas produksi terhadap reverse-engineering app. Kalau ciphertext-nya diutak-atik dikit aja (bukan hasil enkripsi valid), request **ditolak** (AES-GCM itu *authenticated*, bukan cuma "gak kebaca").

Request tanpa header ini, atau ciphertext-nya invalid/gak bisa didekripsi → ditolak di level middleware, gak sempat masuk ke logic endpoint manapun.

## 2. Header wajib tambahan buat route **Protected** — `Authorization: Bearer <token>`

Sebagian endpoint (ditandai **Protected** di masing-masing dokumentasi) butuh header **kedua**, di atas `X-App-Setting`:

```
Authorization: Bearer <session_token>
```

`<session_token>` didapat dari salah satu endpoint login: [`register`](AUTH/REGISTER.md), [`login_otp`](AUTH/LOGIN%20OTP.md), [`login_pin`](AUTH/LOGIN%20PIN.md), atau [`pin/reset`](AUTH/PIN%20RESET.md) — semuanya balikin `token` di response sukses. Token ini **scoped ke 1 customer** (`master_member`), disimpen di `mobile_member_session`, umur **30 hari**.

Server resolve `member_id` dari token ini (dicek exists & belum expired) — **bukan** dari body/param request. Jadi endpoint Protected apapun otomatis cuma bisa akses/ubah data milik akun yang lagi login, gak ada cara akses data member lain lewat endpoint yang sama.

Token gak ada / format salah / gak valid / udah expired:

```json
{ "code": 100, "message": "token tidak ditemukan", "data": null }
```
```json
{ "code": 100, "message": "token tidak valid", "data": null }
```

Endpoint **Publik** (gak ada tanda Protected di dokumentasinya) cukup `X-App-Setting` doang, gak butuh `Authorization`.

## 3. Format response

Semua endpoint balikin bentuk yang sama, **selalu HTTP 200** — sukses/gagal dibedain dari `code` di body (bukan dari HTTP status code):

```json
{ "code": 0, "message": "success", "data": { ... } }
```

- `code: 0` → sukses.
- `code: 100` → gagal (validasi, gak ketemu, dll) — `message` isinya alasan spesifik (bahasa Indonesia), `data` biasanya `null`.

## Daftar modul

| Modul | Keterangan |
|---|---|
| [`AUTH/`](AUTH) | Register, login (OTP & PIN), kelola PIN (create/change/reset) |
| [`ACCOUNT/`](ACCOUNT) | Profil akun, saldo & poin (+riwayat), daftar tier customer yang lagi login |
| [`BANNER/`](BANNER) | Splash, quick action, login sheet, & daftar banner (swipe/popup/promotion/about us) -- scoped per brand |
| [`MENU/`](MENU) | Branch, visit purpose, tree menu + harga + pajak + package, payment method, best seller |
| [`ORDER/`](ORDER) | Promo, calculate (preview), create order, payment status, cancel, history, detail |

**Alur Menu → Order lengkap** (urutan pakai endpoint, cart di sisi client, state order, keterbatasan yang perlu diketahui FE): lihat [`PANDUAN FRONTEND ORDER & MENU.md`](PANDUAN%20FRONTEND%20ORDER%20%26%20MENU.md).
