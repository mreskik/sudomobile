# QR Order - Ketentuan

Dokumen ini ngejelasin **ketentuan/mekanisme QR Order secara menyeluruh** — aturan main yang
berlaku lintas endpoint. Spek request/response tiap endpoint yang beneran makai ketentuan ini
nyusul di file terpisah di folder ini (1 file per endpoint, pola sama kayak
[`../MOBILE/ORDER/KETENTUAN PROMO.md`](../MOBILE/ORDER/KETENTUAN%20PROMO.md) vs
[`../MOBILE/ORDER/CALCULATE.md`](../MOBILE/ORDER/CALCULATE.md)).

**Status: RANCANGAN (2026-09-17)** — belum ada kode/migration. Bagian yang ditandai `[BELUM
DIPUTUSIN]` masih nunggu keputusan; bagian lain udah disepakati di sesi.

## Konsep singkat

QR Order = fasilitas pesan lewat scan QR (di meja/outlet), customer masuk alur pesan **tanpa
install/login app member**. Backend-nya numpang `sudomobile` (1 DB yang sama, `db_sudocore_dev`),
tapi jalur/route-nya terpisah dari API member di [`../MOBILE/`](../MOBILE/README.md).

## Identitas request (bare minimum) — DISEPAKATI 2026-09-17

Setiap request QR Order **minimal** bawa 3 identifier (#1–#3); endpoint yang butuh konteks visit
purpose (menu/order) nambah #4:

| # | Identifier | Nunjuk ke | Cara resolve / validasi |
|---|---|---|---|
| 1 | `db_code` | Penanda database/tenant | **WAJIB dikirim, tapi BELUM DIPAKAI** (keputusan 2026-09-17) — cuma dicek ada/gak kosong, isinya gak divalidasi ke mana pun & gak ngaruh ke query. Sengaja diwajibkan dari sekarang biar kontrak request-nya udah bener sejak awal (client udah terbiasa ngirim), tinggal nyalain validasinya nanti tanpa breaking change. Belum ada kolom `db_code` di tabel mana pun; sekarang cuma ada 1 DB (`db_sudocore_dev`). Referensi yang ada: sudocore2 `/me` nurunin dari 4 huruf pertama username (`sudoadmin` → `SUDO`), `X-App-Setting` sudomobile juga bawa `db_code` tapi sama-sama diabaikan. |
| 2 | `company_code` | `master_company.code` | **Key lookup company + cross-check ke branch** (keputusan 2026-09-17). `master_company.code` sekarang **wajib & unik case-insensitive** (migration sudocore2 `206_alter_master_company_code_unique.sql` — dulunya gak ada constraint & sempet dobel `ASD`, udah dirapihin; Create sudocore2 nge-generate code unik, Update dinormalisasi uppercase + cek unik, lihat `sudocore2/DOKUMENTASI API/MASTER/MASTER COMPANY.md`). Dicocokin **case-insensitive** (`upper()`), gak ketemu → ditolak; ketemu → branch dari #3 **wajib** `company_id`-nya = company ini. |
| 3 | `branch_code` | `master_branch.code` | **Unik global** (`master_branch_code_key`, bukan per company). Jadi **key lookup utama** — dari sini dapet `branch_id`, `company_id`, `brand_id`, `name_qr_order`. |
| 4 | `visit_purpose_code` | `master_visit_purpose.code` | **Kolom BARU** (keputusan 2026-09-17, migration sudocore2 `207_alter_master_visit_purpose_add_code.sql`) — sebelumnya visit purpose gak punya code sama sekali. Unik global case-insensitive, auto-generate dari nama di Create sudocore2 (lihat `sudocore2/DOKUMENTASI API/MASTER/MASTER VISIT PURPOSE.md`). Dicocokin case-insensitive; harus **nyambung ke branch** lewat `master_branch_visit_purpose` (`branch_id` dari #3 + `visit_purpose_id` dari code ini, `is_active = true`, flag channel `[BELUM DIPUTUSIN: flag_qr_order baru atau reuse flag yang ada?]`) → dari situ dapet `menu_template_id`/tax/`order_fee`. Dipakai buat endpoint menu (versi QR Order dari `../MOBILE/MENU/GET VISIT PURPOSE DETAIL.md`, nyusul). |

Aturan turunan:
- Ketiganya wajib ada; salah satu kosong → request ditolak (pesan error per identifier, nyusul
  di spek endpoint).
- `branch_code` gak ketemu → ditolak. Ketemu tapi `company_code` gak cocok → ditolak (bukan
  diem-diem pakai branch yang ketemu). `db_code` **gak** ikut dicocokin (belum dipakai, lihat #1).
- Branch harus **aktif** (`master_branch.status = '1'`) `[BELUM DIPUTUSIN: perlu flag khusus
  "QR order aktif" per branch atau cukup status aktif?]`.
- Cara kirim (query param / body / di-encode di payload QR) `[BELUM DIPUTUSIN]`.
- **Header `X-App-Setting` DIABAIKAN di QR Order** (keputusan 2026-09-17) — gak wajib dikirim,
  kalau dikirim pun gak dibaca. Identitas tenant/company/brand yang di member app dibawa
  `X-App-Setting` (`db_code`/`company_id`/`brand_id`), di sini **digantiin 3 kode di atas**
  (`brand_id` ikut ke-resolve dari `master_branch.brand_id` lewat `branch_code`). Konsekuensi
  implementasi: route QR Order **gak boleh** didaftarin di bawah group `/api` sudomobile yang
  sekarang (`root := app.Group("/api", middleware.AppSetting(...))`, `backend/router.go`) —
  harus group sendiri tanpa middleware itu, mis. `/api/qr-order/...` didaftarin langsung di
  `app`, bukan di `root`.

## Auth & identitas customer

`[BELUM DIPUTUSIN]` — QR Order publik tanpa login member? Atau boleh "nempel" ke member kalau
customer-nya login (ngaruh ke poin/saldo/tier yang semuanya keyed `member_id`)? Kalau publik,
apa identitas minimum yang diminta dari customer (nama? nomor HP?) buat panggilan pesanan/struk.

## Nomor meja

`[BELUM DIPUTUSIN]` — QR per **meja** (nomor meja ikut jadi identitas request, nyambung Master
Table Section sudocore2) atau QR per **outlet** (meja diisi manual/gak ada)?

## Menu yang boleh muncul

- Item yang tampil di QR Order = yang `master_pricelist_detail.qr_order = true` (flag ini udah
  ada & udah dipakai filter menu sudomobile, lihat
  [`../MOBILE/MENU/GET VISIT PURPOSE DETAIL.md`](../MOBILE/MENU/GET%20VISIT%20PURPOSE%20DETAIL.md))
  `[KONFIRMASI: reuse flag ini apa adanya?]`.
- Nama branch yang ditampilin: `master_branch.name_qr_order` (fallback `name` kalau kosong)
  `[KONFIRMASI]`.
- Harga/pajak/package: `[BELUM DIPUTUSIN]` — reuse resolusi yang sama kayak member app
  (`sudomobile/backend/pricing`, DPP-first) atau ada beda?
- Promo: `[BELUM DIPUTUSIN]` — berlaku di QR Order? Kalau iya, channel `apply_to` yang mana
  (`mobile_customer` udah kepakai member app; POS pakai `pos`) — kemungkinan butuh nilai baru.

## Order & pembayaran

`[BELUM DIPUTUSIN]`:
- Numpang tabel `mb_order*` (biar pull ke POS, `orderexpiry`, format `order_number`/
  `payment_number` yang udah ada langsung kepakai) atau tabel sendiri?
- Bayar di tempat (kasir) vs bayar online (QRIS lewat service `payment`, kayak member app)?
- Batas waktu order/pembayaran, dan boleh cancel atau enggak.

## Barrier / validasi (lengkap)

Diisi setelah ketentuan di atas diputusin — format tabel `#` / Barrier / Sumber / Kalau gagal,
sama kayak `KETENTUAN PROMO.md`.

## Beda dari API member (`../MOBILE/`)

Diisi belakangan — yang udah pasti:

| Hal | API member (`../MOBILE/`) | QR Order |
|---|---|---|
| Header `X-App-Setting` | **Wajib** di semua route (`db_code`/`company_id`/`brand_id`, terenkripsi) | **Diabaikan** — gak wajib, gak dibaca (2026-09-17) |
| Identitas tenant/company/branch | `X-App-Setting` (company/brand) + `branch_id` di path/body | 3 kode: `db_code` + `company_code` + `branch_code` |
| Login member | Token session (`Authorization: Bearer`) buat route protected | `[BELUM DIPUTUSIN]` |

## Riwayat

- 2026-09-17 — dokumen dibikin; identitas request (db_code/company_code/branch_code) disepakati,
  temuan DB dicatat, sisanya masih terbuka.
- 2026-09-17 — `X-App-Setting` diputusin DIABAIKAN di QR Order (digantiin 3 kode identitas).
- 2026-09-17 — `db_code` diputusin WAJIB dikirim tapi BELUM DIPAKAI (cuma cek gak kosong).
- 2026-09-17 — `company_code` diputusin jadi key + cross-check; `master_company.code` dirapihin
  jadi wajib & unik case-insensitive (migration sudocore2 206).
- 2026-09-17 — `visit_purpose_code` ditambahin sebagai identifier #4 (visit purpose dicetak di
  QR, bukan dipilih setelah scan); kolom `master_visit_purpose.code` dibikin + auto-generate
  (migration sudocore2 207).
