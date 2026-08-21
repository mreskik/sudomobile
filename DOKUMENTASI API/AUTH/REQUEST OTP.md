# Auth - Request OTP

```
POST /api/auth/request_otp
```

Generate & simpen kode OTP buat 1 `phone_number`. **Dipake bareng buat 3 skenario**, dibedain
lewat `type`:

- `"register"` -- daftar nomor baru, kodenya dipakai di [Register](REGISTER.md).
- `"login"` -- login nomor lama, kodenya dipakai di [Login OTP](LOGIN%20OTP.md).
- `"reset_pin"` -- reset PIN yang lupa (2026-08-21), kodenya dipakai di [Reset PIN](PIN%20RESET.md).

`type` yang diminta cuma boleh dipakai di endpoint pasangannya masing-masing -- gak bisa ketuker
(lihat "Catatan").

Wajib header `X-App-Setting`, sama kayak semua route lain di service ini.

## Request

```json
{
  "phone_number": "62899888700001",
  "type": "register"
}
```

- `phone_number` -- wajib diisi. Sama kayak `check_number`, **gak ada normalisasi** -- dikirim
  apa adanya, dipake apa adanya (jadi tetap harus format `62xxx`, biar nyambung pas dicocokin
  di `register`/`login_otp` nanti).
- `type` -- wajib, **`"register"`**, **`"login"`**, atau **`"reset_pin"`** (persis 3 nilai itu,
  gak ada lain). Nentuin OTP ini boleh dipakai di endpoint mana -- OTP `type=register` gak bisa
  dipakai `login_otp` atau `pin/reset`, dan seterusnya, saling silang (lihat "Catatan").

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "phone_number": "62899888700001",
    "type": "register",
    "expires_in_seconds": 300,
    "expires_at": "2026-08-20T13:42:38.681093+07:00"
  }
}
```

- `expires_in_seconds` -- nilai **tetap** (300), dikirim SEKALI pas response ini, bukan
  angka yang "nurun" tiap detik dari backend. Frontend yang bikin timer/countdown-nya sendiri
  di sisi client (gak nembak API lagi buat nanya sisa berapa detik).
- `expires_at` -- waktu absolut (ISO 8601, ada timezone) kapan OTP ini beneran mati. Lebih
  disaranin dipakai frontend buat itung sisa waktu (`expires_at - waktu sekarang`) daripada
  cuma ngandelin `expires_in_seconds` doang -- kalau app sempet di-*background* terus dibuka
  lagi, ngitung ulang dari `expires_at` tetep akurat, sedangkan ngelanjut countdown dari angka
  terakhir yang "dibekukan" pas app di-background bisa salah.

Kalau `phone_number` kosong:

```json
{ "code": 100, "message": "phone_number wajib diisi", "data": null }
```

Kalau `type` bukan `"register"`/`"login"`/`"reset_pin"` (atau kosong):

```json
{ "code": 100, "message": "type wajib \"register\", \"login\", atau \"reset_pin\"", "data": null }
```

Kalau masih kena **cooldown** (lihat "Catatan" -- belum 5 menit sejak request OTP terakhir buat
`phone_number` ini):

```json
{
  "code": 100,
  "message": "terlalu sering minta otp, coba lagi nanti",
  "data": { "retry_after_seconds": 180 }
}
```

- `retry_after_seconds` -- sisa detik sebelum boleh minta OTP lagi. Dipakai frontend buat
  nunjukin countdown / disable tombol "Kirim OTP", tanpa perlu itung sendiri dari OTP
  sebelumnya atau nembak API lagi buat nanya sisanya.

## Catatan

- **Cooldown 5 menit, GLOBAL per `phone_number`** (2026-08-21) -- bukan per `type`. Sengaja
  global: kalau di-split per `type`, orang tinggal gonta-ganti `type` (`register`→`login`→
  `reset_pin`) buat munculin OTP baru tiap kali, efektif ngelewatin limitnya. Gak ada skenario
  legit yang butuh minta OTP `register` DAN `login` buat nomor yang sama dalam waktu
  berdekatan juga -- `check_number` udah nentuin duluan mau ke alur mana. Dihitung dari
  `created_at` **request OTP terakhir** (lintas `type`), gak peduli udah diverifikasi/dipakai
  atau belum -- ini soal frekuensi request, bukan status OTP-nya. Konstanta `otpCooldown` di
  `backend/modules/auth/auth_handler.go`, kebetulan nilainya sama kayak `otpExpiry` (5 menit) --
  efeknya OTP baru cuma bisa diminta kalau OTP lama udah expired juga, jadi gak akan ada 2 OTP
  valid nempel bareng buat 1 nomor.
- **Kode OTP: 4 digit angka** (`"0000"`-`"9999"`, padded), **expired 5 menit**. Disepakati
  segini buat sekarang, gampang diubah lewat konstanta `otpExpiry` di
  `backend/modules/auth/auth_handler.go` kalau nanti mau diganti.
- **Belum ada provider WA/SMS** -- kode OTP **cuma disimpen ke tabel `mobile_member_otp`** (ERP,
  `sudocore2`) dan di-`log.Println` ke console server. Buat development, kode-nya dicek
  **langsung lewat database**:
  ```sql
  SELECT otp_code FROM mobile_member_otp WHERE phone_number = '...' AND type = '...' ORDER BY id DESC LIMIT 1;
  ```
  **Response API TIDAK pernah ngasih balik kode OTP-nya** -- itu prinsip aja walau sekarang
  belum ada provider (jangan biasain expose OTP lewat response, biar gampang kalau providernya
  udah ada tinggal ganti cara kirimnya doang, gak perlu ubah kontrak API).
- **`type` mengunci OTP ke 1 alur** -- kolom `type` (`register`/`login`/`reset_pin`) di
  `mobile_member_otp` (migration `101`, value `reset_pin` ditambah migration `106`) nyegah OTP
  yang diminta buat 1 tujuan dipakai buat tujuan lain. Contoh: minta OTP `type=register`, terus
  nyoba pakai kode itu buat `login_otp` -> ditolak `"otp tidak ditemukan atau sudah kedaluwarsa"`
  (bukan ketemu tapi ditolak -- emang gak ketemu, karena query-nya ikut filter `type = 'login'`).
- **OTP lama gak dihapus/di-*invalidate* eksplisit** pas ada request baru buat
  `phone_number`+`type` yang sama -- semua baris tetep ada di `mobile_member_otp`. Yang
  nentuin OTP mana yang berlaku itu konsumennya (`register`/`login_otp`/[`pin/reset`](PIN%20RESET.md),
  lewat helper `findValidOTP()`): selalu ambil baris **paling baru** (`ORDER BY id DESC LIMIT 1`)
  yang masih `expires_at > now()` dan `verified_at IS NULL`, **DAN** `type`-nya cocok. Jadi kalau
  user minta OTP 2x buat `type` yang sama, otomatis cuma yang terakhir yang kepake.
- Tabel `mobile_member_otp` dibikin di migration `100`, kolom `type` ditambah di migration
  `101`, value `reset_pin` ditambah ke `CHECK` constraint-nya di migration `106`
  (`sudocore2/cmd/migration`).

## Tervalidasi live (2026-08-20)

- Request `type=register` -> sukses, baris baru di `mobile_member_otp` dengan `type='register'`.
- Request `type` ngasal (`"terserah"`) -> ditolak (pesan versi lama, sebelum `reset_pin`
  ditambah -- lihat catatan di bawah).
- Kode `type=register` dipake langsung di `register` -- sukses (lihat `REGISTER.md`).
- Kode `type=register` (fresh, belum expired) dicoba dipake di `login_otp` -> ditolak `"otp
  tidak ditemukan atau sudah kedaluwarsa"`, bukti isolasi `type` jalan.
- Request `type=login`, kode-nya dipake di `login_otp` -- sukses (lihat `LOGIN OTP.md`).

**⚠️ Belum tervalidasi live** (2026-08-21): `type=reset_pin` dan **cooldown 5 menit** -- `CHECK`
constraint DB buat `reset_pin` udah diupdate & dites langsung lewat `psql` (insert manual
berhasil), query cooldown juga udah dites langsung ke DB (`SELECT created_at ... ORDER BY id DESC
LIMIT 1` balik bener). Tapi keduanya belum dicoba lewat request HTTP beneran ke endpoint
`request_otp`. Baru lolos `go build`/`go vet` dari sisi kode. Update bagian ini kalau udah dites.
