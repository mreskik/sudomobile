# QR Order - Ketentuan

Dokumen ini ngejelasin **ketentuan/mekanisme QR Order secara menyeluruh** — aturan main yang
berlaku lintas endpoint. Spek request/response tiap endpoint yang beneran makai ketentuan ini
nyusul di file terpisah di folder ini (1 file per endpoint, pola sama kayak
[`../MOBILE/ORDER/KETENTUAN PROMO.md`](../MOBILE/ORDER/KETENTUAN%20PROMO.md) vs
[`../MOBILE/ORDER/CALCULATE.md`](../MOBILE/ORDER/CALCULATE.md)).

**Status (2026-09-18): SEMUA 8 endpoint SUDAH JALAN & tervalidasi live** (Create Order, Payment
Status, Get Visit Purpose Detail, Get Payment Method List, Calculate, Order Detail, Get Branch
List, Get Visit Purpose List — lihat file masing-masing). Bagian yang ditandai `[BELUM DIPUTUSIN]`
masih nunggu keputusan (gak nge-block endpoint yang udah ada); bagian lain udah disepakati &
diimplementasi.

## Konsep singkat

QR Order = fasilitas pesan lewat scan QR (di meja/outlet), customer masuk alur pesan **tanpa
install/login app member**. Backend-nya numpang `sudomobile` (1 DB yang sama, `db_sudocore_dev`),
tapi jalur/route-nya terpisah dari API member di [`../MOBILE/`](../MOBILE/README.md).

## Identitas request (bare minimum) — DISEPAKATI 2026-09-17, TIERED 2026-09-18

Ada **4** identifier — **db_code, company_code, branch_code, visit_purpose_code** — tapi
**BUKAN SEMUANYA WAJIB DI SETIAP ENDPOINT** (revisi 2026-09-18, sebelumnya keempatnya wajib di
semua endpoint tanpa kecuali). Sekarang **berjenjang**, dipakai buat 2 skenario QR yang beda:

- **QR-nya udah nentuin semua** (di meja/outlet spesifik, encode 4 kode lengkap) — customer
  langsung ke endpoint transaksional/detail (Create, Calculate, Payment Status, Order Detail, Get
  Visit Purpose Detail, Get Payment Method List), **tetap wajib 4 kode penuh**, gak berubah dari
  keputusan awal.
- **QR-nya cuma nentuin company** (mis. QR generik di depan outlet/website, cuma encode
  `db_code`+`company_code`) — customer PILIH sendiri branch & visit purpose lewat 2 endpoint baru
  (2026-09-18), progresif:
  1. `db_code` + `company_code` (2 kode) → [`GET BRANCH LIST.md`](./ORDER/01%20GET%20BRANCH%20LIST.md)
     — daftar branch di company itu (`qrorder.ResolveCompany()`).
  2. `db_code` + `company_code` + `branch_code` (3 kode, `branch_code` dari hasil #1) →
     [`GET VISIT PURPOSE LIST.md`](./ORDER/02%20GET%20VISIT%20PURPOSE%20LIST.md) — daftar visit
     purpose valid buat branch itu (`qrorder.ResolveBranch()`).
  3. Baru abis itu customer punya 4 kode lengkap, lanjut ke endpoint transaksional/detail yang
     sama kayak skenario pertama.

Jadi `visit_purpose_code` (dan `branch_code`) **kondisional** — wajib buat endpoint yang
"ngerjain sesuatu" (liat menu, hitung, order), belum wajib di 2 endpoint discovery yang justru
buat NEMUIN kode itu duluan. Tabel di bawah tetap berlaku buat SEMUA level (`ResolveCompany()`
cuma jalanin baris #1-2, `ResolveBranch()` #1-3, `Resolve()` penuh #1-4) — barrier/urutan/pesan
error per identifier PERSIS SAMA di ketiganya, cuma titik berhentinya beda:

| # | Identifier | Nunjuk ke | Cara resolve / validasi |
|---|---|---|---|
| 1 | `db_code` | Penanda database/tenant | **WAJIB dikirim, tapi BELUM DIPAKAI** (keputusan 2026-09-17) — cuma dicek ada/gak kosong, isinya gak divalidasi ke mana pun & gak ngaruh ke query. Sengaja diwajibkan dari sekarang biar kontrak request-nya udah bener sejak awal (client udah terbiasa ngirim), tinggal nyalain validasinya nanti tanpa breaking change. Belum ada kolom `db_code` di tabel mana pun; sekarang cuma ada 1 DB (`db_sudocore_dev`). Referensi yang ada: sudocore2 `/me` nurunin dari 4 huruf pertama username (`sudoadmin` → `SUDO`), `X-App-Setting` sudomobile juga bawa `db_code` tapi sama-sama diabaikan. |
| 2 | `company_code` | `master_company.code` | **Key lookup company + cross-check ke branch** (keputusan 2026-09-17). `master_company.code` sekarang **wajib & unik case-insensitive** (migration sudocore2 `206_alter_master_company_code_unique.sql` — dulunya gak ada constraint & sempet dobel `ASD`, udah dirapihin; Create sudocore2 nge-generate code unik, Update dinormalisasi uppercase + cek unik, lihat `sudocore2/DOKUMENTASI API/MASTER/MASTER COMPANY.md`). Dicocokin **case-insensitive** (`upper()`), gak ketemu → ditolak; ketemu → branch dari #3 **wajib** `company_id`-nya = company ini. |
| 3 | `branch_code` | `master_branch.code` | **Unik global** (`master_branch_code_key`, bukan per company). Jadi **key lookup utama** — dari sini dapet `branch_id`, `company_id`, `brand_id`, `name_qr_order`. |
| 4 | `visit_purpose_code` | `master_visit_purpose.code` | **Kolom BARU** (keputusan 2026-09-17, migration sudocore2 `207_alter_master_visit_purpose_add_code.sql`) — sebelumnya visit purpose gak punya code sama sekali. Unik global case-insensitive, auto-generate dari nama di Create sudocore2 (lihat `sudocore2/DOKUMENTASI API/MASTER/MASTER VISIT PURPOSE.md`). **WAJIB di semua request** (bukan opsional). Dicocokin case-insensitive; harus **nyambung ke branch** lewat `master_branch_visit_purpose` (`branch_id` dari #3 + `visit_purpose_id` dari code ini, `is_active = true`, **`flag_mobile_customer = true`** — keputusan 2026-09-17: reuse flag channel member app, **gak** bikin `flag_qr_order` baru; konsekuensinya visit purpose yang dinyalain buat member app otomatis kebuka juga buat QR Order, 1 saklar buat 2 channel — kalau nanti perlu dibedain, tinggal nambah flag terpisah) → dari situ dapet `menu_template_id`/tax/`order_fee`. Dipakai buat endpoint menu (versi QR Order dari `../MOBILE/MENU/GET VISIT PURPOSE DETAIL.md`, nyusul). |

Aturan turunan:
- Identifier yang RELEVAN buat endpoint itu wajib ada; salah satu kosong → request ditolak (pesan
  error per identifier, nyusul di spek endpoint). Get Branch List cuma butuh #1-2, Get Visit
  Purpose List #1-3, sisanya (endpoint transaksional/detail) #1-4 penuh — lihat tabel skenario di
  atas.
- Urutan resolve: `company_code` → company; `branch_code` → branch (kalau diminta level ini),
  **wajib** `company_id`-nya = company itu; `visit_purpose_code` → visit purpose (kalau diminta
  level ini), **wajib** nyambung ke branch itu lewat `master_branch_visit_purpose`
  (`flag_mobile_customer = true`, `is_active = true`). Gagal di langkah mana pun → ditolak (bukan
  diem-diem pakai yang ketemu). `db_code` **gak** ikut dicocokin (belum dipakai, lihat #1).
- Branch harus **aktif** (`master_branch.status = '1'`) `[BELUM DIPUTUSIN: perlu flag khusus
  "QR order aktif" per branch atau cukup status aktif?]`.
- **Cara kirim**: **query param di SEMUA endpoint**, `GET` maupun `POST` (keputusan 2026-09-17 —
  seragam, body `POST` cuma isi cart/pembayaran; 1 middleware di backend nge-resolve 4 kode buat
  semua route). Format payload di dalam QR-nya sendiri (URL apa adanya vs di-encode)
  `[BELUM DIPUTUSIN]`.
- **Salah satu gak valid → SELALU error** (keputusan 2026-09-17), gak ada fallback/default —
  pesan per langkah, lihat tabel error di spek endpoint.
- **Header `X-App-Setting` DIABAIKAN di QR Order** (keputusan 2026-09-17) — gak wajib dikirim,
  kalau dikirim pun gak dibaca. Identitas tenant/company/brand yang di member app dibawa
  `X-App-Setting` (`db_code`/`company_id`/`brand_id`), di sini **digantiin 3 kode di atas**
  (`brand_id` ikut ke-resolve dari `master_branch.brand_id` lewat `branch_code`). Konsekuensi
  implementasi: route QR Order **gak boleh** didaftarin di bawah group `/api` sudomobile yang
  sekarang (`root := app.Group("/api", middleware.AppSetting(...))`, `backend/router.go`) —
  harus group sendiri tanpa middleware itu. **Prefix-nya `/qr-order`, BUKAN `/api/qr-order`**
  (kebukti pas implementasi Create Order, 2026-09-17 — `AppSetting` ke-`Use()` di Fiber v3
  matching PREFIX PATH "/api", jadi apa pun yang diawali `/api` tetep ketangkep walau
  didaftarin lewat `app.Group()` yang beda; solusinya pindah ke luar prefix `/api` sama sekali).

## Auth & identitas customer

**Keputusan 2026-09-17: TAMU, tanpa login member.** Semua endpoint QR Order publik. Identitas
customer = `order_name` (**wajib** di Create, nama kolom `mb_order.order_name` — DIRENAME dari
`customer_name` di migration `211`, disamain sama `tr_order.order_name` yang udah ada di POS) +
`customer_phone_number` (opsional), disimpen di `mb_order` (`member_id` **NULL** — kolomnya dibikin
nullable, lihat migration di [`CREATE ORDER.md`](./ORDER/06%20CREATE%20ORDER.md)). Konsekuensi: order QR **gak
nyambung** ke poin/saldo/tier/promo member (semua keyed `member_id`) — diterima buat v1. Kalau
nanti mau "nempel ke member kalau login", itu tambahan mode auth di Create, bukan mengubah yang
ini.

**Akses ke order yang udah dibuat** (detail/status bayar/QR ulang): **cukup `order_number`**
(keputusan 2026-09-17, tanpa token/no HP) + cek `order_source = 'qr'` & branch-nya cocok sama
`branch_code`. Risiko `order_number` ketebak (format `NO`+branch_code+YmdHis+2 digit) **diterima**;
opsi perketat nanti: `order_token` dari Create.

## Nomor meja

`[BELUM DIPUTUSIN]` — QR per **meja** (nomor meja ikut jadi identitas request, nyambung Master
Table Section sudocore2) atau QR per **outlet** (meja diisi manual/gak ada)?

## Menu yang boleh muncul

- Item yang tampil di QR Order = yang `master_pricelist_detail.qr_order = true` (flag ini udah
  ada & udah dipakai filter menu sudomobile, lihat
  [`../MOBILE/MENU/GET VISIT PURPOSE DETAIL.md`](../MOBILE/MENU/GET%20VISIT%20PURPOSE%20DETAIL.md))
  — reuse apa adanya (spek [`GET VISIT PURPOSE DETAIL.md`](./ORDER/03%20GET%20VISIT%20PURPOSE%20DETAIL.md)).
- Nama branch yang ditampilin: `master_branch.name_qr_order` (fallback `name` kalau kosong), plus
  `address`.
- Harga/pajak/package: **reuse persis** resolusi member app (`sudomobile/backend/pricing`,
  `calculateOrder()`, DPP-first) — keputusan 2026-09-17, bukan implementasi baru. Payment method:
  filter yang sama (gateway-only, scope branch+visit purpose), spek [`GET PAYMENT METHOD LIST.md`](./ORDER/04%20GET%20PAYMENT%20METHOD%20LIST.md).
- **Promo: BELUM ADA di QR Order v1** (keputusan 2026-09-17) — `use_promo_ids` ditolak kalau
  dikirim (`promo belum didukung di QR Order`). Alasan: barrier member_type & min_point butuh
  `member_id` (tamu gak punya), dan channel `master_promo_apply_to` belum punya nilai buat QR
  (cuma `mobile_customer`/`pos`). Ditambah belakangan kalau perlu (butuh nilai channel baru + cabang
  "tanpa member" di `pricing.ResolvePromo()`).

## Order & pembayaran

**Keputusan 2026-09-17:**
- **Numpang tabel `mb_order*`** yang sama kayak member app, dibedain kolom
  **`mb_order.order_source`** (**SELESAI**, migration sudocore2 209 — `mobile` buat member app,
  **`qr`** buat QR Order). `member_id` nullable **SELESAI** (migration 208), `order_name`
  **SELESAI** (migration 210, DIRENAME dari `customer_name` di migration 211 biar sama kayak
  `tr_order.order_name` di POS). Create Order beneran udah jalan, lihat
  [`CREATE ORDER.md`](./ORDER/06%20CREATE%20ORDER.md). `table_number` masih `[BELUM DIPUTUSIN]` — DIBUANG
  dari scope Create Order v1 (bukan cuma nyusul, keputusan eksplisit gak dipaksain sampai jelas
  skemanya).
- **Bayar online (QRIS)** lewat service `payment` — alur, `mb_order_payment_request`, sync status,
  `expired_at`, semuanya **sama persis** member app; gak ada bayar di kasir di v1.
- Pull ke POS, job `orderexpiry`, format `order_number`/`payment_number` → otomatis kepakai karena
  tabelnya sama. POS bedain lewat `order_source` — **SELESAI** (2026-09-17): APIANDORDER
  `GetPending()` udah ngalirin kolom ini, `MobileOrderPullServices.php` udah baca dari payload
  (bukan hardcode lagi). Detail & hasil tes di `CREATE ORDER.md`.
- **Cancel** oleh customer `[BELUM DIPUTUSIN]` (member app punya `order/:order_number/cancel`
  sebelum bayar — mau ada versi QR-nya, atau cukup dibiarin expired?).
- **Polling status bayar — SELESAI** (2026-09-17, keputusan: endpoint `payment-status` TERPISAH,
  lebih ringan dari `ORDER DETAIL.md` yang belum dibikin) — lihat
  [`PAYMENT STATUS.md`](./ORDER/07%20PAYMENT%20STATUS.md). "Kepemilikan"-nya `order_source='qr'` + branch
  cocok (bukan token/member), REUSE `SyncPaymentStatus()` member app apa adanya.

## Barrier / validasi (lengkap)

Barrier **identitas request** — dijalanin di SEMUA endpoint QR Order, urut, berhenti di yang
pertama gagal (semua `code: 100`):

| # | Barrier | Sumber | Kalau gagal |
|---|---|---|---|
| 1 | 4 query param ada & gak kosong | `db_code`, `company_code`, `branch_code`, `visit_purpose_code` | `"<param> wajib diisi"` |
| 2 | Company ketemu | `master_company.code` (case-insensitive) | `"company tidak ditemukan"` |
| 3 | Branch ketemu | `master_branch.code` (case-insensitive) | `"branch tidak ditemukan"` |
| 4 | Branch milik company itu | `master_branch.company_id` = #2 | `"branch bukan milik company ini"` |
| 5 | Branch aktif | `master_branch.status = '1'` | `"branch tidak aktif"` |
| 6 | Visit purpose ketemu & nyambung ke branch | `master_visit_purpose.code` (case-insensitive) + baris `master_branch_visit_purpose` (`branch_id` #3, `visit_purpose_id`, `flag_mobile_customer = true`, `is_active = true`) | `"visit purpose tidak ditemukan"` |

(`db_code` cuma kena #1.) Barrier per-endpoint (cart, payment method, branch buka/online,
kepemilikan order) ada di spek masing-masing.

## Beda dari API member (`../MOBILE/`)

Diisi belakangan — yang udah pasti:

| Hal | API member (`../MOBILE/`) | QR Order |
|---|---|---|
| Header `X-App-Setting` | **Wajib** di semua route (`db_code`/`company_id`/`brand_id`, terenkripsi) | **Diabaikan** — gak wajib, gak dibaca (2026-09-17) |
| Identitas tenant/company/branch/visit purpose | `X-App-Setting` (company/brand) + `branch_id`/`visit_purpose_id` di path | 4 kode: `db_code` + `company_code` + `branch_code` + `visit_purpose_code`, wajib semua |
| Login member | Token session (`Authorization: Bearer`) buat route protected | **Gak ada** — semua publik, customer = tamu (`order_name` wajib, `member_id` NULL) |
| Akses order lama | Cek kepemilikan `member_id` | Cukup `order_number` (+ `order_source = 'qr'`, branch cocok) |
| Promo | `use_promo_ids`, 14 barrier | **Belum ada** (v1), `use_promo_ids` ditolak |
| Tabel order | `mb_order*`, `order_source = 'mobile'` | `mb_order*` yang sama, `order_source = 'qr'` |
| Pembayaran | QRIS via service `payment` | Sama persis |
| `order_type` | hardcode `takeaway` | `[KONFIRMASI]` dari `kiosk_mode` visit purpose, fallback `dinein` |

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
- 2026-09-17 — `visit_purpose_code` diputusin **WAJIB di semua request** (4 identifier, bukan 3+1);
  cek channel-nya reuse `master_branch_visit_purpose.flag_mobile_customer`, gak bikin flag baru.
- 2026-09-17 — 4 kode dikirim sebagai **query param `GET`**; salah satu gak valid → selalu error.
  Spek endpoint pertama ditulis: `GET VISIT PURPOSE DETAIL.md`.
- 2026-09-17 — Diputusin: customer = **tamu** (`member_id` nullable + `customer_name`), akses
  order **cukup `order_number`**, **belum ada promo**, 4 kode **query param juga di `POST`**,
  tabel **`mb_order*`** dengan `order_source = 'qr'`. Spek ditulis: `GET PAYMENT METHOD
  LIST.md`, `CALCULATE.md`, `CREATE ORDER.md` (termasuk DDL draft), `ORDER DETAIL.md`.
- 2026-09-17 — `mb_order.member_id` jadi nullable (migration sudocore2 208, **SELESAI diterapkan**
  + 3 fix Go pendukung di sudomobile/APIANDORDER, tervalidasi live). `mb_order.order_source`
  ditambahin (migration sudocore2 209, **SELESAI**, sisi sudomobile doang — Create member app
  nulis `'mobile'`); nama kolom & nilainya disamain final ke `order_source`/`'qr'` (bukan
  `order_from`/`'qrorder'` kayak draft awal), ikut vocab yang udah dianggarkan komentar skema
  POS.
- 2026-09-17 — Pull ke POS disesuaikan biar `order_source` ngalir: APIANDORDER `GetPending()`
  (`mobileorder_service.go`+`_model.go`) nambahin kolom ini, POS
  `MobileOrderPullServices::processOrder()` gak lagi hardcode `'mobile'`. Tervalidasi live lewat
  `get_pending` (JSON balikin `order_source` bener); sisi POS-nya dicek syntax + pola konsisten,
  belum lewat `artisan mobile-order:pull` beneran.
- 2026-09-17 — **Create Order QR Order SELESAI diimplementasi & tervalidasi live end-to-end**
  (migration `210` nambahin `customer_name`, package baru `backend/modules/qrorder/` buat resolusi
  4 kode, handler di package `order` biar reuse fungsi member app langsung). `table_number`
  eksplisit DIBUANG dari scope v1. Ketemu 1 hal penting soal routing Fiber v3 pas implementasi:
  prefix WAJIB `/qr-order`, **bukan** `/api/qr-order` (`middleware.AppSetting` ke-`Use()` matching
  prefix path `/api` di level `app`, nangkep apa pun yang diawali situ walau didaftarin lewat
  `app.Group()` yang beda) — detail di `CREATE ORDER.md`.
- 2026-09-17 — **Payment Status (polling) SELESAI diimplementasi & tervalidasi live**, termasuk
  jalur settlement beneran (`status` `pending`→`paid`, idempotency kebukti jalan) — lihat
  [`PAYMENT STATUS.md`](./ORDER/07%20PAYMENT%20STATUS.md). "Polling status bayar" di atas gak lagi
  `[BELUM DIPUTUSIN]`.
- 2026-09-17 — **Get Visit Purpose Detail SELESAI diimplementasi & tervalidasi live** — lihat
  [`GET VISIT PURPOSE DETAIL.md`](./ORDER/03%20GET%20VISIT%20PURPOSE%20DETAIL.md). `qrorder.Context`
  diperluas bawa `CompanyName`/`BranchName`/`BranchAddress`/`VisitPurposeName` sekalian (gak
  nambah round-trip), dipakai buat blok `company`/`branch`/`visit_purpose` di response endpoint
  ini.
- 2026-09-17 — **Get Payment Method List + Calculate SELESAI diimplementasi & tervalidasi live**
  — lihat `GET PAYMENT METHOD LIST.md`/`CALCULATE.md`. 4 dari 5 endpoint QR Order sekarang jalan.
- 2026-09-17 — **Order Detail SELESAI diimplementasi & tervalidasi live** — lihat
  [`ORDER DETAIL.md`](./ORDER/08%20ORDER%20DETAIL.md). `resolveOrderDetailCore()` diekstrak dari `GetDetail()`
  member app (item/package/payment, reuse `SyncPaymentStatus()` apa adanya); header & kepemilikan
  (`order_source='qr'` + branch cocok) tetep query & struct terpisah, pola sama kayak
  `qrOrderOwnerRow` di Payment Status. **Keenam endpoint QR Order yang direncanakan sekarang
  SELESAI semua.** Ketemu sekalian akar masalah "`air` sempat stuck" (dicatat di README.md) — 5
  proses `air`/`main.exe` sudomobile numpuk dari sesi 2026-09-13 s/d 2026-09-16 yang gak pernah
  dimatiin pas ganti hari, rebutan file lock/port; proses yang megang port 96 udah diganti yang
  seger, sisanya (elevated, gak kebunuh dari shell biasa) udah gak megang port jadi gak ganggu
  tapi idealnya tetep dibersihin manual dari Task Manager.
- 2026-09-17 — `mb_order.customer_name` di-**RENAME** jadi **`order_name`** (migration sudocore2
  211) — disamain sama nama kolom yang udah ada di POS, `tr_order.order_name` (persis sebelahan
  sama `tr_order.order_source` yang udah lebih dulu disamain namanya di `209`), biar pas jalur pull
  `mb_order` → `tr_order` digarap field-nya udah cocok tanpa mapping/alias. Konsumen Go
  (`qrCreateOrderRequest`/`qrCreateOrderResult`/`qrOrderDetailHeader`/`qrOrderDetailResult`, field
  `CustomerName`→`OrderName`, JSON `customer_name`→`order_name`) diubah bareng migration ini,
  tervalidasi live ulang (Create + Order Detail). Entry riwayat sebelum ini yang masih nyebut
  `customer_name` **dibiarin apa adanya** (catatan sejarah keputusan waktu itu), bukan diedit.
- 2026-09-17 — Risiko terbuka "order QR kesenggol job tier/point" (dicatat di `CREATE ORDER.md`
  bagian "Belum dikerjain") **DICEK, AMAN**. Ditelusuri jalur `mb_order.member_id NULL` →
  `pos_order.member_id NULL` (beneran ngalir lewat pull APIANDORDER/POS/push data, dikonfirmasi
  baca kode, bukan asumsi) → semua konsumen (`pointcheck`, `membertierevaluation`, `TierSpending`
  sudobarber & sudomobile) udah punya guard `member_id IS NOT NULL` atau `member_id = ?` per-member
  yang otomatis gak match `NULL`. Detail lengkap + sitasi file:baris di `CREATE ORDER.md`.
- 2026-09-17 — Gap "mapping `order_name` ke POS belum jalan" (dari rename `211`) **DIBERESIN**.
  `APIANDORDER` `GetPending()` nambahin `mo.order_name` ke payload (nullable, gak di-`COALESCE`);
  POS `MobileOrderPullServices.php` jadi `$order['order_name'] ?? $order['member_name'] ?? ''`.
  Tervalidasi live lewat `get_pending` beneran (bukan cuma syntax check) — order QR & order member
  app dua-duanya dites, hasil bener (detail di `CREATE ORDER.md`). `artisan mobile-order:pull`
  end-to-end POS tetep belum, sama kayak fix `order_source` sebelumnya.
- 2026-09-18 — **`branch_code`/`visit_purpose_code` jadi KONDISIONAL** (sebelumnya 4 kode wajib
  MUTLAK di semua endpoint, gak ada pengecualian). 2 endpoint baru buat alur "QR company doang"
  (customer belum tau branch/visit purpose sama sekali): **Get Branch List** (`db_code`+
  `company_code` doang, `qrorder.ResolveCompany()`) dan **Get Visit Purpose List** (+`branch_code`,
  `qrorder.ResolveBranch()`). Package `qrorder` di-refactor (`Context` sekarang embed
  `BranchContext` yang embed `CompanyContext`, Go struct embedding) — SEMUA endpoint lama
  (Create/Calculate/PaymentStatus/OrderDetail/GetVisitPurposeDetail/GetPaymentMethodList) **GAK
  ADA YANG BERUBAH KODENYA SAMA SEKALI**, field promotion bikin `qrCtx.BranchID` dst tetep jalan
  apa adanya; `Resolve()` sendiri byte-for-byte sama urutan cek & pesannya kayak sebelum refactor
  (query company/branch cuma dipindah ke helper `resolveCompanyRow()`/`resolveBranchRow()` yang
  dipakai bareng, bukan ditulis ulang logic-nya). Filter Get Branch List:
  `master_branch_setting.flag_online_service_mobile_customer = true AND master_branch.status = '1'`
  (persis filter [`../MOBILE/MENU/GET BRANCH LIST.md`](../MOBILE/MENU/GET%20BRANCH%20LIST.md)
  member app, ditambah scope 1 company). Detail lengkap + tervalidasi live di
  [`GET BRANCH LIST.md`](./ORDER/01%20GET%20BRANCH%20LIST.md) &
  [`GET VISIT PURPOSE LIST.md`](./ORDER/02%20GET%20VISIT%20PURPOSE%20LIST.md). **Delapan endpoint QR
  Order sekarang SELESAI.**
