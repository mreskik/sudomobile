# Auth - Register

```
POST /api/auth/register
```

Daftarin nomor HP baru jadi `master_member`, **sekaligus** login (langsung terbitin session
token) -- gak ada langkah "verify OTP" terpisah dari "bikin akun", 1 kali call aja. Dipanggil
setelah `request_otp` (lihat `REQUEST OTP.md`).

Wajib header `X-App-Setting`, sama kayak semua route lain di service ini.

## Request

```json
{
  "phone_number": "62899888700001",
  "name": "Budi Testing",
  "otp": "9704"
}
```

- `phone_number` -- wajib, format `62xxx` (gak dinormalisasi, sama kayak `check_number`).
- `name` -- wajib.
- `otp` -- wajib, kode 4 digit hasil `request_otp` **`type=register`** buat nomor yang sama
  (OTP `type=login` gak bisa dipakai di sini -- lihat `REQUEST OTP.md`).

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "token": "c13d3634358fd50329cb86eb6932a18c4797d13fdac34754e5cf5e7b63298c62",
    "member": {
      "id": 20,
      "code": "MOB0001",
      "name": "Budi Testing",
      "phone_number": "62899888700001",
      "has_pin": false
    }
  }
}
```

- `member.has_pin` (2026-08-21) -- selalu `false` di sini, gak nge-query DB (member baru aja dibuat, mustahil udah punya PIN). Sekadar konsisten sama bentuk response endpoint auth lain (`login_otp`/`login_pin`/`pin/reset`) yang juga punya field ini.

- `token` -- session token, umur **30 hari** dari sekarang (`mobile_member_session.expires_at`).
  Dipake buat header auth di endpoint yang butuh login (belum ada -- `CustomerAuth` middleware
  belum diimplementasi).
- `member.code` -- prefix **`MOB`** + sequence 4 digit (`MOB0001`, `MOB0002`, ...), KHUSUS
  buat member yang daftar sendiri lewat mobile app -- beda dari prefix nama member_type/`GEN`
  yang dipakai kalau diinput manual di ERP. Sequence-nya dihitung dari code existing yang
  udah mulai `MOB`, jadi gak nabrak sama code lain.

Error yang mungkin balik (semua tetep HTTP `200`, `code: 100`):

| `message` | Kapan |
| --- | --- |
| `phone_number wajib diisi` / `name wajib diisi` / `otp wajib diisi` | Field kosong |
| `nomor sudah terdaftar` | `phone_number` udah ada di `master_member` -- di-cek ULANG di sini (bukan cuma percaya hasil `check_number` sebelumnya), jaga-jaga race condition |
| `otp tidak ditemukan atau sudah kedaluwarsa` | Gak ada baris `mobile_member_otp` buat nomor ini yang masih valid (`expires_at > now()`, `verified_at IS NULL`) |
| `otp salah` | Ada OTP valid buat nomor ini, tapi kode-nya gak cocok |

## Catatan

- **Transaksi 1 kesatuan** (`BeginTx`/`Commit`/`Rollback`, pola sama kayak
  `member_point_adjustment` di `sudocore2`): insert `master_member` baru → tandain
  `mobile_member_otp.verified_at` (biar OTP itu gak bisa dipakai ulang) → insert
  `mobile_member_session`. Kalau salah satu gagal, semuanya di-rollback -- gak ada kondisi
  "member kebuat tapi session gagal" atau sebaliknya.
- **OTP dicek ulang persis** (`otp.OTPCode != req.OTP`) SETELAH ambil baris paling baru yang
  valid -- kalau kodenya salah tapi baris OTP-nya sendiri masih ada & valid, pesannya `"otp
  salah"` (bukan "tidak ditemukan"), biar user tau bedanya "kodenya emang keliru" vs "OTP-nya
  udah kadaluwarsa/gak pernah minta".
- **`master_member` kolom lain** (`member_type_id`, `contact_name`, `email`, dst) **gak
  diisi** -- member hasil register mobile app cuma punya `code`/`name`/`phone_number`/
  `is_active`. Kalau nanti butuh field lain, itu didiskusikan/ditambah belakangan (kemungkinan
  lewat endpoint update profile terpisah, bukan di sini).

## Tervalidasi live (2026-08-20)

- `register` pakai OTP **salah** -> `"otp salah"`, gak ada yang kebuat.
- `register` pakai OTP **bener** -> sukses, member baru (`MOB0001`), `mobile_member_otp`
  ke-mark `verified_at`, `mobile_member_session` kebuat dengan `expires_at` = +30 hari persis.
- `register` ULANG pakai nomor yang sama (walau OTP yang dikirim itu-itu lagi) -> `"nomor
  sudah terdaftar"`, ketauan lewat cek ulang, bukan lewat OTP re-use check.
- `check_number` buat nomor yang barusan register -> `is_registered: true`, bukti konsisten
  sama endpoint lain.
- Data test dihapus lagi abis verifikasi (`master_member`/`mobile_member_otp`/
  `mobile_member_session`), gak ninggalin sampah.
