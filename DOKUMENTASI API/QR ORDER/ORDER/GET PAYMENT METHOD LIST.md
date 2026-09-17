# QR Order - Get Payment Method List

**Status: SELESAI & tervalidasi live (2026-09-17).**

```
GET /qr-order/payment-method?db_code=SUDO&company_code=SUDO&branch_code=SBJ&visit_purpose_code=DIN
```

**Publik** — tanpa `Authorization`, tanpa `X-App-Setting`. Versi QR Order dari
[`../MOBILE/MENU/GET PAYMENT METHOD LIST.md`](../../MOBILE/MENU/GET%20PAYMENT%20METHOD%20LIST.md):
daftar payment method yang bisa dipakai buat kombinasi branch + visit purpose ini — **gateway-only**
(punya `payment_gateway_code`; QR Order sama kayak member app: online, gak ada kasir yang mungutin
cash) dan di-scope branch + visit purpose (hormatin `flag_all_branch`/`flag_all_visitpurpose`).
**Bedanya cuma cara nunjuk branch & visit purpose** (4 kode di query, bukan `:branch_id`/
`:visit_purpose_id` di path); query & filter-nya **sama persis**, direncanakan reuse fungsi yang
sama (`pricing.ResolvePaymentMethod()`/handler list yang udah ada).

## Request

4 kode identitas, semua wajib, query param — aturan & urutan cek-nya sama persis
[`GET VISIT PURPOSE DETAIL.md`](./GET%20VISIT%20PURPOSE%20DETAIL.md#request) (gak diulang di sini).
Gak ada body.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    { "id": 1, "name": "QRIS", "code": "QRA", "color_theme": "feaw" }
  ]
}
```

Sama persis versi member app. Kombinasi yang gak punya payment method cocok → `data: []`, bukan
error. `id`-nya yang dikirim balik sebagai `payment_method_id` di
[`CREATE ORDER.md`](./CREATE%20ORDER.md).

## Error

Cuma error 4 kode identitas — tabel lengkapnya di
[`GET VISIT PURPOSE DETAIL.md`](./GET%20VISIT%20PURPOSE%20DETAIL.md#error). Semua `code: 100`, HTTP `200`.

## Catatan implementasi

Query-nya (`resolvePaymentMethodList()`) diekstrak dari `GetList()` member app
(`paymentmethod_handler.go`, 2026-09-17) biar dipakai bareng TANPA duplikasi — handler QR
(`paymentmethod_qr_handler.go`) tinggal `qrorder.Resolve()` lalu panggil fungsi yang sama.

## Tervalidasi live (2026-09-17)

Branch 51 (`SBE`)/company `SUDO`/visit purpose 7 (`ASD`) — data yang sama dipakai endpoint QR
Order lain:
- Identitas bener → `[{"id":1,"name":"QRIS","code":"QRA","color_theme":"feaw"}]`, sama persis
  yang dipakai `payment_method_id` di Create Order.
- `db_code` kosong → `"db_code wajib diisi"`. `company_code` salah → `"company tidak
  ditemukan"`.
- Semua 4 kode lowercase → tetap sukses, hasil identik.
