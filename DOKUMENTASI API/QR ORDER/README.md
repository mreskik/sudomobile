# QR Order API

Folder ini buat API fasilitas **QR Order** di `sudomobile` — customer scan QR (mis. di
meja/outlet), langsung masuk alur pesan tanpa install/login app member. Jalur terpisah dari API
member di [`../MOBILE/`](../MOBILE/README.md), tapi 1 backend & 1 DB yang sama.

**Status (2026-09-17): SEMUA 6 endpoint SUDAH JALAN & tervalidasi live** (Create Order, Payment
Status, Get Visit Purpose Detail, Get Payment Method List, Calculate, Order Detail).

## Daftar dokumen

- [`KETENTUAN QR ORDER.md`](./KETENTUAN%20QR%20ORDER.md) — ketentuan/mekanisme menyeluruh
  (identitas request `db_code`/`company_code`/`branch_code`, auth, meja, menu, order &
  pembayaran) + daftar hal yang belum diputusin. **Baca ini dulu.**
- Spek per endpoint, di folder [`ORDER/`](./ORDER/) (1 file per endpoint, pola sama kayak
  `../MOBILE/`) — **semua SELESAI & tervalidasi live**:
  - [`ORDER/GET VISIT PURPOSE DETAIL.md`](./ORDER/GET%20VISIT%20PURPOSE%20DETAIL.md) —
    `GET /qr-order/visit-purpose/detail?…` — **SELESAI & tervalidasi live (2026-09-17)** — pohon
    menu + harga + pajak + package (versi QR Order dari
    [`../MOBILE/MENU/GET VISIT PURPOSE DETAIL.md`](../MOBILE/MENU/GET%20VISIT%20PURPOSE%20DETAIL.md)).
  - [`ORDER/GET PAYMENT METHOD LIST.md`](./ORDER/GET%20PAYMENT%20METHOD%20LIST.md) — `GET /qr-order/payment-method?…`
    — **SELESAI & tervalidasi live (2026-09-17)**.
  - [`ORDER/CALCULATE.md`](./ORDER/CALCULATE.md) — `POST /qr-order/calculate?…` — **SELESAI & tervalidasi
    live (2026-09-17)** — preview harga keranjang, tanpa promo.
  - [`ORDER/CREATE ORDER.md`](./ORDER/CREATE%20ORDER.md) — `POST /qr-order/create-order?…` — **SELESAI &
    tervalidasi live (2026-09-17)** — order tamu ke `mb_order*` (`order_source = 'qr'`) + QRIS.
  - [`ORDER/PAYMENT STATUS.md`](./ORDER/PAYMENT%20STATUS.md) — `GET /qr-order/order/:order_number/payment-status?…`
    — **SELESAI & tervalidasi live (2026-09-17)** — polling status bayar, reuse `SyncPaymentStatus()`.
  - [`ORDER/ORDER DETAIL.md`](./ORDER/ORDER%20DETAIL.md) — `GET /qr-order/order/:order_number?…` — **SELESAI &
    tervalidasi live (2026-09-17)** — struk + status bayar live + QR ulang.

  Semua endpoint bawa 4 kode identitas yang sama di query string (`?db_code=&company_code=&branch_code=&visit_purpose_code=`).

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
  [`ORDER/CREATE ORDER.md`](./ORDER/CREATE%20ORDER.md#riwayat-perubahan-skema-migration-sudocore2).
- 2026-09-17 — Spek per endpoint dipindah ke subfolder [`ORDER/`](./ORDER/) (`KETENTUAN QR ORDER.md`
  & `README.md` tetep di sini) — link internal antar file (sama folder) gak berubah, link balik ke
  `KETENTUAN QR ORDER.md` & link ke `../MOBILE/...` di dalam file yang pindah disesuaikan jadi 1
  level lebih dalam.
