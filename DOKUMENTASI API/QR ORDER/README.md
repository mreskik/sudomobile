# QR Order API

Folder ini buat API fasilitas **QR Order** di `sudomobile` — customer scan QR (mis. di
meja/outlet), langsung masuk alur pesan tanpa install/login app member. Jalur terpisah dari API
member di [`../MOBILE/`](../MOBILE/README.md), tapi 1 backend & 1 DB yang sama.

**Status (2026-09-18): SEMUA 8 endpoint SUDAH JALAN & tervalidasi live** (Create Order, Payment
Status, Get Visit Purpose Detail, Get Payment Method List, Calculate, Order Detail, Get Branch
List, Get Visit Purpose List).

## Daftar dokumen

- [`KETENTUAN QR ORDER.md`](./KETENTUAN%20QR%20ORDER.md) — ketentuan/mekanisme menyeluruh
  (identitas request `db_code`/`company_code`/`branch_code`, auth, meja, menu, order &
  pembayaran) + daftar hal yang belum diputusin. **Baca ini dulu.**
- Spek per endpoint, di folder [`ORDER/`](./ORDER/) (1 file per endpoint, pola sama kayak
  `../MOBILE/`) — **semua SELESAI & tervalidasi live**:
  - [`ORDER/GET VISIT PURPOSE DETAIL.md`](./ORDER/03%20GET%20VISIT%20PURPOSE%20DETAIL.md) —
    `GET /qr-order/visit-purpose/detail?…` — **SELESAI & tervalidasi live (2026-09-17)** — pohon
    menu + harga + pajak + package (versi QR Order dari
    [`../MOBILE/MENU/GET VISIT PURPOSE DETAIL.md`](../MOBILE/MENU/GET%20VISIT%20PURPOSE%20DETAIL.md)).
  - [`ORDER/GET PAYMENT METHOD LIST.md`](./ORDER/04%20GET%20PAYMENT%20METHOD%20LIST.md) — `GET /qr-order/payment-method?…`
    — **SELESAI & tervalidasi live (2026-09-17)**.
  - [`ORDER/05%20CALCULATE.md`](./ORDER/05%20CALCULATE.md) — `POST /qr-order/calculate?…` — **SELESAI & tervalidasi
    live (2026-09-17)** — preview harga keranjang, tanpa promo.
  - [`ORDER/CREATE ORDER.md`](./ORDER/06%20CREATE%20ORDER.md) — `POST /qr-order/create-order?…` — **SELESAI &
    tervalidasi live (2026-09-17)** — order tamu ke `mb_order*` (`order_source = 'qr'`) + QRIS.
  - [`ORDER/PAYMENT STATUS.md`](./ORDER/07%20PAYMENT%20STATUS.md) — `GET /qr-order/order/:order_number/payment-status?…`
    — **SELESAI & tervalidasi live (2026-09-17)** — polling status bayar, reuse `SyncPaymentStatus()`.
  - [`ORDER/ORDER DETAIL.md`](./ORDER/08%20ORDER%20DETAIL.md) — `GET /qr-order/order/:order_number?…` — **SELESAI &
    tervalidasi live (2026-09-17)** — struk + status bayar live + QR ulang.
  - [`ORDER/GET BRANCH LIST.md`](./ORDER/01%20GET%20BRANCH%20LIST.md) — `GET /qr-order/branch-list?…`
    — **SELESAI & tervalidasi live (2026-09-18)** — daftar branch 1 company, cuma butuh 2 kode
    (`db_code`+`company_code`).
  - [`ORDER/GET VISIT PURPOSE LIST.md`](./ORDER/02%20GET%20VISIT%20PURPOSE%20LIST.md) — `GET
    /qr-order/visit-purpose/list?…` — **SELESAI & tervalidasi live (2026-09-18)** — daftar visit
    purpose 1 branch, cuma butuh 3 kode (+`branch_code`).

  Endpoint transaksional/detail bawa 4 kode identitas penuh di query string
  (`?db_code=&company_code=&branch_code=&visit_purpose_code=`); 2 endpoint discovery di atas
  (Get Branch List/Get Visit Purpose List) cuma butuh sebagian — lihat
  [`KETENTUAN QR ORDER.md`](./KETENTUAN%20QR%20ORDER.md#identitas-request-bare-minimum--disepakati-2026-09-17-tiered-2026-09-18).

## Riwayat

- 2026-09-17 — folder dibikin, ketentuan awal ditulis (belum ada kode).
- 2026-09-17 — Create Order diimplementasi & tervalidasi live end-to-end (migration sudocore2
  `208`-`210`, package baru `backend/modules/qrorder/`, handler di package `order`). Detail
  lengkap + 1 gotcha routing Fiber v3 yang kepentok pas dev di `CREATE ORDER.md`.
- 2026-09-17 — Payment Status (polling) diimplementasi & tervalidasi live, termasuk jalur
  settlement beneran (idempotency kebukti jalan). Detail di `PAYMENT STATUS.md`.
- 2026-09-17 — Get Visit Purpose Detail diimplementasi & tervalidasi live (`resolveVisitPurposeDetail()`
  diekstrak dari `GetDetail()` member app biar dipakai bareng). Detail di `GET VISIT PURPOSE
  DETAIL.md`.
- 2026-09-17 — Get Payment Method List + Calculate diimplementasi & tervalidasi live
  (`resolvePaymentMethodList()` diekstrak dari `GetList()` member app; Calculate reuse
  `calculateOrder()` apa adanya, `memberID=0`). 4 dari 5 endpoint QR Order sekarang jalan — cuma
  Order Detail (struk) yang masih rancangan.
- 2026-09-17 — Order Detail diimplementasi & tervalidasi live (`resolveOrderDetailCore()`
  diekstrak dari `GetDetail()` member app buat item/package/payment, header & ownership check
  tetep terpisah karena kolomnya genuinely beda). **Semua 6 endpoint QR Order SELESAI.** Ketemu &
  dibereskan juga: 5 proses `air`/`main.exe` sudomobile numpuk dari sesi dev 2026-09-13 s/d
  2026-09-16 yang gak pernah dimatiin (rebutan file lock/port, itu penyebab "air stuck" yang
  keliatan pas tes Calculate) — proses hari-hari lama yang jalan elevated gak kebunuh (access
  denied) tapi udah gak megang port jadi gak ganggu; proses yang megang port 96 dimatiin & diganti
  yang seger sesuai binary hasil build terakhir.
- 2026-09-17 — `mb_order.customer_name` di-rename jadi `order_name` (migration sudocore2 211,
  disamain sama `tr_order.order_name` di POS) — detail di riwayat `KETENTUAN QR ORDER.md` dan
  [`ORDER/CREATE ORDER.md`](./ORDER/06%20CREATE%20ORDER.md#riwayat-perubahan-skema-migration-sudocore2).
- 2026-09-17 — Spek per endpoint dipindah ke subfolder [`ORDER/`](./ORDER/) (`KETENTUAN QR ORDER.md`
  & `README.md` tetep di sini) — link internal antar file (sama folder) gak berubah, link balik ke
  `KETENTUAN QR ORDER.md` & link ke `../MOBILE/...` di dalam file yang pindah disesuaikan jadi 1
  level lebih dalam.
- 2026-09-18 — **Get Branch List + Get Visit Purpose List** diimplementasi & tervalidasi live —
  4 kode identitas gak lagi wajib MUTLAK di semua endpoint, sekarang berjenjang (buat QR yang cuma
  encode company, customer pilih branch lalu visit purpose sendiri). Package `qrorder`
  di-refactor jadi 3 level resolve (`ResolveCompany`/`ResolveBranch`/`Resolve`, Go struct
  embedding) — endpoint LAMA gak ada yang berubah kodenya. **Delapan endpoint QR Order
  SELESAI.** Detail di riwayat `KETENTUAN QR ORDER.md`.
