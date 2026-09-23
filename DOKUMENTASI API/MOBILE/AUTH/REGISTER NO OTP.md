# Auth - Register (Tanpa OTP)

```
POST /api/auth/register-no-otp
```

**BARU (2026-09-23).** Versi [Register](REGISTER.md) **TANPA verifikasi OTP** — daftarin nomor HP baru jadi `master_member`, sekaligus login (langsung terbitin session token), tanpa perlu `request_otp` duluan sama sekali. Endpoint **TERPISAH** dari Register biasa — Register yang lama **TIDAK diubah**, OTP-nya tetap wajib seperti sebelumnya.

Wajib header `X-App-Setting`, sama kayak semua route lain di service ini.

## ⚠️ Konsekuensi keamanan

**Endpoint ini TIDAK memverifikasi kepemilikan nomor HP sama sekali.** Beda dari Register (OTP) yang minimal membuktikan pemanggil bisa menerima kode di nomor itu, di sini siapa pun bisa mendaftarkan nomor siapa saja tanpa bukti apa pun — server cuma cek format (digit + panjang minimal) dan belum terdaftar.

Endpoint ini publik total, **tanpa guard tambahan** (disepakati eksplisit saat dirancang, 2026-09-23) — dipakai atas kebutuhan produk yang memang butuh onboarding tanpa menunggu OTP.

**Rencana ke depan (belum diimplementasikan)**: pasang **Cloudflare Turnstile** (captcha) di endpoint ini sebagai barier anti-bot/anti-spam minimal, mengingat tidak ada verifikasi OTP yang menghambat pemanggilan berulang. Dicatat sebagai TODO, bukan hal yang dianggap "belum lengkap tapi sengaja ditunda tanpa rencana".

## Request

```json
{
  "phone_number": "62899888700001",
  "name": "Budi Testing",
  "pin": "123456"
}
```

- `phone_number` — wajib. **Divalidasi format**: harus semua digit (`^[0-9]{10,}$`), minimal **10 karakter**. Tidak ada normalisasi (sama konvensi `check_number`/Register) dan tidak wajib prefix `62` — cuma dicek angka semua + panjang minimal.
- `name` — wajib.
- `pin` — **wajib** (2026-09-23), 6 digit angka. **Beda dari [Register](REGISTER.md)** yang tidak punya field ini sama sekali (PIN diset belakangan lewat Create PIN) — di sini PIN langsung diset SEKALIAN pas register, lihat "Catatan" di bawah kenapa.
- **Tidak ada field `otp`** — beda dari [Register](REGISTER.md).

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "token": "c13d3634358fd50329cb86eb6932a18c4797d13fdac34754e5cf5e7b63298c62",
    "member": {
      "id": 21,
      "code": "MOB0002",
      "name": "Budi Testing",
      "phone_number": "62899888700001",
      "has_pin": true
    }
  }
}
```

Bentuk response **SAMA PERSIS** [Register](REGISTER.md) — `token` (session, umur 30 hari) + `member`.

- `member.has_pin` — SELALU `true` (literal, gak perlu di-query) — **beda dari Register (OTP) yang selalu `false`** — PIN wajib diisi & langsung diset pas register di endpoint ini.
- `member.code` — prefix **`MOB`** + sequence 4 digit, **fungsi generator yang SAMA** dipakai Register (OTP) — sequence-nya nyambung 1 hitungan, **tidak dibedakan** apakah member itu daftar lewat jalur OTP atau tanpa OTP.
- `master_member.member_type_id` selalu `3` (Customer, `auth.MemberTypeCustomerID`) — sama seperti Register.
- Kolom lain (`contact_name`, `email`, dst) tidak diisi — sama seperti Register.

Error yang mungkin balik (semua tetep HTTP `200`, `code: 100`):

| `message` | Kapan |
| --- | --- |
| `phone_number wajib diisi` / `name wajib diisi` / `pin wajib diisi` | Field kosong |
| `phone_number tidak valid` | `phone_number` bukan semua digit, atau kurang dari 10 karakter |
| `pin harus 6 digit angka` | `pin` bukan 6 digit angka |
| `nomor sudah terdaftar` | `phone_number` udah ada `master_member`-nya |

## Catatan

- **Transaksi 1 kesatuan**: insert `master_member` baru → insert `mobile_member_session` → insert `mobile_member_pin`. Kalau salah satu gagal, semuanya di-rollback. **Beda dari Register**: tidak ada langkah "tandai OTP `verified_at`" — jalur ini memang tidak pernah menyentuh `mobile_member_otp` sama sekali.
- **Kenapa `pin` wajib di sini, padahal Register (OTP) tidak punya field ini?** Endpoint ini tidak ada OTP untuk verifikasi ulang identitas kalau PIN mau diset belakangan. Mumpung member baru masih memegang token session yang fresh langsung dari Register, PIN diminta sekalian di sini.
- **Guard duplikasi nomor** — sama persis Register: dicek `phone_number` belum ada di `master_member` sebelum insert (bukan cuma percaya `check_number` sebelumnya).
- **Kenapa endpoint terpisah, bukan `otp` dibikin opsional di Register?** Supaya Register yang sudah ada tidak melemah secara diam-diam — validasi OTP-nya tetap ketat seperti sebelumnya, tidak ada ambiguitas "kirim `otp: ""` itu sengaja skip atau lupa isi".
- Insert PIN di sini **TIDAK** memanggil handler `CreatePin` yang sudah ada — itu mengambil `member_id` dari session token via middleware `Auth`, yang belum jalan di titik request Register (belum ada token). Insert `mobile_member_pin` dilakukan langsung di dalam transaksi yang sama dengan insert `master_member`/`mobile_member_session`.
- Implementasi: `sudomobile/backend/modules/auth/auth_handler.go` (`RegisterNoOTP()`), `sudomobile/backend/modules/auth/generators.go` (`validPhoneNumber`).

## Status

Baru dibuat (2026-09-23), belum ada validasi live tercatat. Cloudflare Turnstile **belum dipasang** — lihat peringatan keamanan di atas.
