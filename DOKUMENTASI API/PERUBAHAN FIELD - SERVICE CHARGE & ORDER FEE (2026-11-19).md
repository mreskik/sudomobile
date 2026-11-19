# Perubahan Field — Service Charge & Order Fee (2026-11-19)

**Breaking change API publik sudomobile.** Rename field harga per item (`dpp`/`net_dpp` →
`net_price`/`dpp`) DAN field baru Service Charge/Order Fee yang sebelumnya selalu `0`, sekarang
beneran dihitung. **Request TIDAK berubah di endpoint manapun** — cuma field response yang
berubah, lihat tabel di bawah.

## Daftar perubahan field

| Path Dokumentasi | Endpoint | Field Sebelum | Field Sesudah |
|---|---|---|---|
| [`MOBILE/ORDER/CALCULATE.md`](MOBILE/ORDER/CALCULATE.md) | `POST /api/order/calculate` | `dpp`, `net_dpp` (per item) | `net_price`, `dpp` (per item) |
| | | — | `service_charge_id`, `service_charge_rate`, `service_charge_amount` (per item, baru) |
| | | — | `total_service_charge`, `order_fee` (header, baru — `order_fee` dulu ada tapi selalu `"0.00"`) |
| [`MOBILE/ORDER/CREATE ORDER.md`](MOBILE/ORDER/CREATE%20ORDER.md) | `POST /api/order/create-order` | `dpp`, `net_dpp` (per item) | `net_price`, `dpp` (per item) |
| | | — | `service_charge_id`, `service_charge_rate`, `service_charge_amount` (per item, baru) |
| | | — | `total_service_charge`, `order_fee` (header, baru) |
| | | `platform_fee` (header) | **dihapus** |
| [`MOBILE/ORDER/ORDER DETAIL.md`](MOBILE/ORDER/ORDER%20DETAIL.md) | `GET /api/order/:order_number` | `dpp`, `net_dpp` (per item + package) | `net_price`, `dpp` (per item + package) |
| | | — | `service_charge_id`, `service_charge_rate`, `service_charge_amount` (per item + package, baru) |
| | | — | `total_service_charge`, `order_fee` (header, baru) |
| [`MOBILE/MENU/GET BEST SELLER.md`](MOBILE/MENU/GET%20BEST%20SELLER.md) | `GET .../best-seller` (varian dengan harga) | `dpp`, `net_dpp` (per item) | `net_price`, `dpp` (per item) |
| | | — | `service_charge_id`, `service_charge_rate`, `service_charge_amount` (per item, baru) |
| [`QR ORDER/ORDER/05 CALCULATE.md`](QR%20ORDER/ORDER/05%20CALCULATE.md) | `POST /qr-order/calculate` | sama seperti `CALCULATE.md` | sama seperti `CALCULATE.md` |
| [`QR ORDER/ORDER/06 CREATE ORDER.md`](QR%20ORDER/ORDER/06%20CREATE%20ORDER.md) | `POST /qr-order/create-order` | sama seperti `CREATE ORDER.md` | sama seperti `CREATE ORDER.md` |
| [`QR ORDER/ORDER/08 ORDER DETAIL.md`](QR%20ORDER/ORDER/08%20ORDER%20DETAIL.md) | `GET /qr-order/order/:order_number` | sama seperti `ORDER DETAIL.md` | sama seperti `ORDER DETAIL.md` |

**Request**: tidak berubah di semua endpoint di atas — client tetap kirim payload yang sama
(`branch_id`, `items[].menu_id/qty/notes/packages`, dst). Service charge dan order fee dihitung
server dari config visit purpose, bukan dari input client.

**Tidak terdampak** (field-nya tidak berubah sama sekali, sengaja tidak masuk tabel di atas):
[`MOBILE/MENU/GET VISIT PURPOSE DETAIL.md`](MOBILE/MENU/GET%20VISIT%20PURPOSE%20DETAIL.md) dan
[`QR ORDER/ORDER/03 GET VISIT PURPOSE DETAIL.md`](QR%20ORDER/ORDER/03%20GET%20VISIT%20PURPOSE%20DETAIL.md)
— respons API-nya sama persis seperti sebelumnya, cuma ada catatan internal yang diperbarui di
dalamnya.

## Detail lengkap (formula, contoh JSON, hasil live-test)

Lihat [`MOBILE/ORDER/CALCULATE.md`](MOBILE/ORDER/CALCULATE.md) sebagai sumber kebenaran utama —
semua dokumen lain di tabel atas tinggal merujuk ke situ untuk detail formula perhitungan dan
contoh response lengkap.
