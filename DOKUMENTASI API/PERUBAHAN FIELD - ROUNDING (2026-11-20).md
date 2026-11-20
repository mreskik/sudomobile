# Perubahan Field — Rounding (2026-11-20)

**Breaking change API publik sudomobile.** Field baru `grand_total` dan `rounding_amount` di level
order (header), menyusul fitur rounding (pembulatan nominal order) yang sekarang diterapkan.
**Request TIDAK berubah di endpoint manapun** — cuma field response yang berubah, lihat tabel di
bawah.

`grand_total` = akumulasi item + `total_service_charge` + `order_fee`, SEBELUM dibulatkan (ini
yang SEBELUMNYA namanya `total_billing`). `rounding_amount` = selisih pembulatan (tergantung
setting rounding branch, bisa `"0.00"`). `total_billing` TETAP ADA, TETAP nilai final yang harus
ditagih/ditampilkan ke customer sebagai "Total Bayar" — cuma nilainya sekarang `grand_total +
rounding_amount`, bisa beda dari `grand_total` kalau branch-nya pakai rounding.

## Daftar perubahan field

| Path Dokumentasi | Endpoint | Field Sebelum | Field Sesudah |
|---|---|---|---|
| [`MOBILE/ORDER/CALCULATE.md`](MOBILE/ORDER/CALCULATE.md) | `POST /api/order/calculate` | `total_billing` (header) | `grand_total`, `rounding_amount`, `total_billing` (header) |
| [`MOBILE/ORDER/CREATE ORDER.md`](MOBILE/ORDER/CREATE%20ORDER.md) | `POST /api/order/create-order` | `total_billing` (header) | `grand_total`, `rounding_amount`, `total_billing` (header) |
| [`MOBILE/ORDER/ORDER DETAIL.md`](MOBILE/ORDER/ORDER%20DETAIL.md) | `GET /api/order/:order_number` | `total_billing` (header) | `grand_total`, `rounding_amount`, `total_billing` (header) |
| [`MOBILE/ORDER/ORDER HISTORY.md`](MOBILE/ORDER/ORDER%20HISTORY.md) | `GET /api/order/history` | `total_billing` (per baris) | `grand_total`, `rounding_amount`, `total_billing` (per baris) |
| [`QR ORDER/ORDER/05 CALCULATE.md`](QR%20ORDER/ORDER/05%20CALCULATE.md) | `POST /qr-order/calculate` | sama seperti `CALCULATE.md` | sama seperti `CALCULATE.md` |
| [`QR ORDER/ORDER/06 CREATE ORDER.md`](QR%20ORDER/ORDER/06%20CREATE%20ORDER.md) | `POST /qr-order/create-order` | sama seperti `CREATE ORDER.md` | sama seperti `CREATE ORDER.md` |
| [`QR ORDER/ORDER/08 ORDER DETAIL.md`](QR%20ORDER/ORDER/08%20ORDER%20DETAIL.md) | `GET /qr-order/order/:order_number` | sama seperti `ORDER DETAIL.md` | sama seperti `ORDER DETAIL.md` |

**Request**: tidak berubah di semua endpoint di atas — client tetap kirim payload yang sama.
`grand_total`/`rounding_amount` dihitung server dari `master_branch_setting` (branch), bukan dari
input client.

**Tidak terdampak** (field-nya tidak berubah sama sekali, sengaja tidak masuk tabel di atas):
[`MOBILE/MENU/GET VISIT PURPOSE DETAIL.md`](MOBILE/MENU/GET%20VISIT%20PURPOSE%20DETAIL.md) dan
[`QR ORDER/ORDER/03 GET VISIT PURPOSE DETAIL.md`](QR%20ORDER/ORDER/03%20GET%20VISIT%20PURPOSE%20DETAIL.md)
— endpoint preview menu, tidak pernah menghitung/mengembalikan total order sama sekali, jadi tidak
ada `grand_total`/`total_billing`/`rounding_amount` di response-nya baik sebelum maupun sesudah
perubahan ini.

## Detail lengkap (formula, contoh JSON, hasil live-test)

Lihat [`MOBILE/ORDER/CALCULATE.md`](MOBILE/ORDER/CALCULATE.md#formula-rounding-2026-11-20) sebagai
sumber kebenaran utama — semua dokumen lain di tabel atas tinggal merujuk ke situ untuk detail
formula perhitungan dan contoh response lengkap.
