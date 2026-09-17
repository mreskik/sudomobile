# sudomobile

Backend buat aplikasi mobile customer (member point/saldo, order, promo/discount, dll).

## Modul & Endpoint

Detail lengkap tiap endpoint (request/response, error case, catatan implementasi) ada di **`DOKUMENTASI API/MOBILE/<NAMA MODUL>/`**. Buat alur Menu→Order lengkap (cara pakai endpoint secara berurutan, state order, keterbatasan yang perlu diketahui FE), lihat [`DOKUMENTASI API/MOBILE/PANDUAN FRONTEND ORDER & MENU.md`](DOKUMENTASI%20API/MOBILE/PANDUAN%20FRONTEND%20ORDER%20%26%20MENU.md).

| Modul                   | Endpoint                                                            | Dokumentasi                                             |
| ----------------------- | ------------------------------------------------------------------- | ------------------------------------------------------- |
| **Auth** (publik)       | `check_number`, `request_otp`, `register`, `login_otp`, `login_pin` | [`DOKUMENTASI API/MOBILE/AUTH/`](DOKUMENTASI%20API/MOBILE/AUTH)       |
| **Auth** (protected)    | `pin/create`, `pin/change`, `pin/reset`, `logout`                   | [`DOKUMENTASI API/MOBILE/AUTH/`](DOKUMENTASI%20API/MOBILE/AUTH)       |
| **Account** (protected) | `account/me` (`GET`+`PUT`), `account/balance(/history)`, `account/point(/history)`, `account/tier-list`, `account/tier-spending`, `account/photo` | [`DOKUMENTASI API/MOBILE/ACCOUNT/`](DOKUMENTASI%20API/MOBILE/ACCOUNT) |
| **Banner** (publik)     | `banner` -- scoped per `brand_id`                                   | [`DOKUMENTASI API/MOBILE/BANNER/`](DOKUMENTASI%20API/MOBILE/BANNER)   |
| **Branch** (publik)     | `branch` -- filter flag_online_service_mobile_customer, gak di-scope brand | [`DOKUMENTASI API/MOBILE/MENU/`](DOKUMENTASI%20API/MOBILE/MENU)   |
| **Branch** (publik)     | `branch/:branch_id/visit-purpose` -- filter flag_mobile_customer  | [`DOKUMENTASI API/MOBILE/MENU/`](DOKUMENTASI%20API/MOBILE/MENU)   |
| **Branch** (publik)     | `branch/:branch_id/visit-purpose/:visit_purpose_id` -- tree menu + pajak + package (selesai 3 tahap) | [`DOKUMENTASI API/MOBILE/MENU/`](DOKUMENTASI%20API/MOBILE/MENU)   |
| **Branch** (publik)     | `branch/:branch_id/visit-purpose/:visit_purpose_id/payment-method` -- gateway-only, scoped branch+visit_purpose | [`DOKUMENTASI API/MOBILE/MENU/`](DOKUMENTASI%20API/MOBILE/MENU)   |
| **Menu** (publik)       | `menu/best-seller`, `branch/:branch_id/best-seller`, `branch/:branch_id/visit-purpose/:visit_purpose_id/best-seller` -- 30 hari terakhir, sumber mb_order doang | [`DOKUMENTASI API/MOBILE/MENU/`](DOKUMENTASI%20API/MOBILE/MENU) |
| **Order** (protected)   | `order/calculate` -- preview breakdown harga/pajak (DPP-first) + promo, baca-only | [`DOKUMENTASI API/MOBILE/ORDER/`](DOKUMENTASI%20API/MOBILE/ORDER)     |
| **Order** (protected)   | `branch/:branch_id/visit-purpose/:visit_purpose_id/promo` -- daftar promo eligible | [`DOKUMENTASI API/MOBILE/ORDER/`](DOKUMENTASI%20API/MOBILE/ORDER)     |
| **Order** (protected)   | `order/create-order` (`POST`) -- save order + trigger payment gateway (QRIS) | [`DOKUMENTASI API/MOBILE/ORDER/`](DOKUMENTASI%20API/MOBILE/ORDER)     |
| **Order** (protected)   | `order/:order_number/payment-status` -- polling status pembayaran | [`DOKUMENTASI API/MOBILE/ORDER/`](DOKUMENTASI%20API/MOBILE/ORDER)     |
| **Order** (protected)   | `order/:order_number/cancel` -- batalin order sebelum bayar (race-guard aware) | [`DOKUMENTASI API/MOBILE/ORDER/`](DOKUMENTASI%20API/MOBILE/ORDER)     |
| **Order** (protected)   | `order/history` -- riwayat order milik member yang login, header doang | [`DOKUMENTASI API/MOBILE/ORDER/`](DOKUMENTASI%20API/MOBILE/ORDER)     |
| **Order** (protected)   | `order/:order_number` (`GET`) -- detail lengkap + QR ulang kalau masih pending | [`DOKUMENTASI API/MOBILE/ORDER/`](DOKUMENTASI%20API/MOBILE/ORDER)     |

## Background Job

Proses yang jalan sendiri di belakang layar (bukan endpoint HTTP), dokumentasinya di **[`DOKUMENTASI BACKGROUND JOB/`](DOKUMENTASI%20BACKGROUND%20JOB)** (pola yang sama kayak `sudocore2`).

## Menjalankan

```
go run main.go
```

Default port `96` (`APP_PORT` di `.env`).
