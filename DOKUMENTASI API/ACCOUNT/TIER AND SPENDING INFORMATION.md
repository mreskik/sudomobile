# Account - Tier and Spending Information

Posisi tier member yang lagi login + progress spending **periode berjalan sekarang** + jadwal evaluasi berikutnya — digabung 1 endpoint (2026-08-21) karena ketiganya satu concern yang sama, basisnya sama-sama dari [MASTER MEMBER TIER SETTING.md](../../../sudocore2/DOKUMENTASI%20API/MASTER/MASTER%20MEMBER%20TIER%20SETTING.md) di ERP.

```
GET /api/account/tier-spending
Authorization: Bearer <token>
```

**Protected** — `member_id` dari session token, sama pola kayak [ME.md](ME.md).

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "type_evaluation": "week",
    "next_evaluation": "2026-08-24",
    "period_start": "2026-08-17",
    "period_end": "2026-08-21",
    "spending_total": "0.00",
    "tier": {
      "level": 1,
      "name": "Bronze",
      "style_template": null
    }
  }
}
```

| Field | Tipe | Keterangan |
|---|---|---|
| `type_evaluation` | string | `"week"` atau `"month"` — alias `master_member_tier_setting.type`, biar jelas basis spending di bawah ini dihitung mingguan atau bulanan |
| `next_evaluation` | date (`YYYY-MM-DD`) | Tanggal evaluasi tier **berikutnya** (bisa hari ini sendiri, kalau hari ini kebetulan cocok jadwal). Lihat "Cara hitung `next_evaluation`" |
| `period_start` | date (`YYYY-MM-DD`) | Awal periode spending berjalan. Lihat "Cara hitung `period_start`" |
| `period_end` | date (`YYYY-MM-DD`) | Selalu **hari ini** |
| `spending_total` | string (numeric) | `SUM(pos_order.total_billing)` member ini, `status='paid'`, dalam rentang `period_start`–`period_end`. `"0.00"` kalau gak ada transaksi |
| `tier.level`/`tier.name`/`tier.style_template` | — | Posisi tier member **saat ini** (`master_member.tier_level`, `LEFT JOIN master_member_tier_setting_detail`) — sama bentuk & sumbernya kayak `tier` yang dulu sempet ada di [ME.md](ME.md) (sekarang udah dipindah ke sini) |

Error yang mungkin balik (semua tetep HTTP `200`, `code: 100`):

| `message` | Kapan |
|---|---|
| `belum ada setting tier` | `master_member_tier_setting` belum pernah di-`PUT` sama sekali |
| `setting tier belum lengkap (type_week_day kosong)` / `(type_month_day kosong)` | `type` udah diisi tapi field pasangannya kosong (data gak konsisten) |

## Cara hitung `period_start` (spending periode berjalan)

**Beda dari window yang dipakai background job [`membertierevaluation`](../../../sudocore2/DOKUMENTASI%20BACKGROUND%20JOB/MEMBER%20TIER%20EVALUATION.md)** — job itu ngitung window **mundur** (trailing 7 hari/1 bulan dari hari evaluasi) buat nentuin naik/turun tier. `period_start` di sini di-anchor ke `type_week_day`/`type_month_day` sebagai **awal periode**, sampai **hari ini** — beda konsep, buat kebutuhan beda (progress display real-time vs evaluasi terjadwal).

- **`type = "week"`**: tanggal terdekat ke belakang (**termasuk hari ini** kalau kebetulan cocok) yang nama harinya sama kayak `type_week_day`. Contoh: `type_week_day: "monday"`, hari ini Jumat → `period_start` = Senin minggu ini.
- **`type = "month"`**: tanggal `type_month_day` **bulan ini**, kalau tanggal hari ini **udah nyampe/lewat** `type_month_day`. Kalau **belum nyampe**, tanggal `type_month_day` **bulan kemarin**. Contoh: `type_month_day: 20`, hari ini tanggal `5` → `period_start` = tanggal `20` bulan lalu.

## Cara hitung `next_evaluation`

Kebalikan `period_start` — nyari **maju**, bukan mundur:

- **`type = "week"`**: tanggal terdekat ke **depan** (**termasuk hari ini** kalau kebetulan cocok) yang nama harinya sama kayak `type_week_day`.
- **`type = "month"`**: tanggal `type_month_day` **bulan ini**, kalau tanggal hari ini **belum lewat** `type_month_day`. Kalau **udah lewat**, tanggal `type_month_day` **bulan depan**.

Ini murni informasional (buat app nunjukin "evaluasi tier berikutnya: 24 Agustus") — **bukan** waktu presisi kapan background job jalan (job-nya sendiri jalan **tiap hari jam 01:00**, cuma beneran ngapa-ngapain di tanggal yang match, lihat [MEMBER TIER EVALUATION.md](../../../sudocore2/DOKUMENTASI%20BACKGROUND%20JOB/MEMBER%20TIER%20EVALUATION.md)).

## Catatan

- **`tier` dipindah dari [ME.md](ME.md) ke sini** (2026-08-21) — awalnya sempet ada di `/me`, dipindah biar `/me` murni data profil statis, sementara tier/spending/evaluasi (yang emang saling terkait & lebih sering di-refresh) ngumpul di 1 endpoint ini.
- Cuma pakai `master_member_tier_setting` (**global**, 1 baris) — gak ada per-member override periode/jadwal.
- `period_start`/`period_end` inklusif dua-duanya — `order_out` difilter `>= period_start` dan `< period_end + 1 hari`.
- `tier.name`/`tier.style_template` bisa `null` kalau admin belum pernah setup `master_member_tier_setting_detail` buat level yang lagi ditempatin member (`LEFT JOIN`, bukan `INNER JOIN` — biar response tetap muncul, cuma dua field itu yang kosong).
- Mau daftar **SEMUA** tier (bukan cuma posisi sekarang) buat roadmap/"road to next tier"? Lihat [TIER LIST.md](TIER%20LIST.md).

## Tervalidasi

- **Logic periode & query** (2026-08-21) — tervalidasi manual: config live sekarang `type=week`/`type_week_day=monday`, hari ini Jumat `2026-08-21` → `period_start` seharusnya `2026-08-17` (Senin minggu ini) dan `next_evaluation` seharusnya `2026-08-24` (Senin depan), dua-duanya dicocokin manual lewat `psql` dan bener. Query spending & tier jalan tanpa error. `go build`/`go vet` bersih.

**⚠️ Belum tervalidasi lewat HTTP request** — belum sempat dicoba lewat request HTTP beneran (server dev butuh restart), dan belum ada data order asli buat mastiin angka `spending_total` yang bukan nol. Update bagian ini kalau udah dites.
