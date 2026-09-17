# QR Order - Calculate

**Status: SELESAI & tervalidasi live (2026-09-17).**

```
POST /qr-order/calculate?db_code=SUDO&company_code=SUDO&branch_code=SBJ&visit_purpose_code=DIN
Content-Type: application/json
```

**Publik** — tanpa `Authorization`, tanpa `X-App-Setting` (beda dari versi member app yang
PROTECTED karena butuh identitas member buat promo — di QR Order **belum ada promo**, jadi gak ada
alasan wajib login). Preview breakdown harga/pajak isi keranjang **sebelum** order disubmit,
baca-only, gak insert apa pun. Versi QR Order dari
[`../MOBILE/ORDER/CALCULATE.md`](../../MOBILE/ORDER/CALCULATE.md) — body **sama persis** kayak
[`CREATE ORDER.md`](./CREATE%20ORDER.md) minus field pembayaran/identitas, dan logic hitungnya
**fungsi yang sama** (`calculateOrder()`/`pricing.CalculateLine()`, DPP-first) — preview di
keranjang gak pernah beda sama yang beneran kesimpen.

## Request

4 kode identitas wajib di **query param** (keputusan 2026-09-17: seragam buat semua endpoint QR
Order, `POST` sekalipun — body cuma isi cart), aturan & urutan cek sama persis
[`GET VISIT PURPOSE DETAIL.md`](./GET%20VISIT%20PURPOSE%20DETAIL.md#request).

Body:
```json
{
  "items": [
    {
      "menu_id": 109,
      "qty": 2,
      "notes": "less ice",
      "packages": [
        { "package_id": 18, "selections": [{ "menu_package_id": 31, "qty": 1 }] }
      ]
    }
  ]
}
```

- **Gak ada `branch_id`/`visit_purpose_id`** di body — udah dari 4 kode di query (kalau dikirim,
  diabaikan).
- `items[].menu_id`/`qty`/`notes`/`packages[]` — **sama persis** versi member app (cuma identitas +
  qty, server resolve ulang harga/pajak/package dari DB; `menu_id` & `package_id`/`menu_package_id`
  diambil dari [`GET VISIT PURPOSE DETAIL.md`](./GET%20VISIT%20PURPOSE%20DETAIL.md)).
- **`use_promo_ids` TIDAK diterima** (keputusan 2026-09-17: promo belum ada di QR Order v1) —
  kalau dikirim & gak kosong → ditolak `"promo belum didukung di QR Order"`, bukan diem-diem
  diabaikan (biar FE gak salah kira diskonnya kepake).

## Response

Sama persis versi member app — lihat
[`../MOBILE/ORDER/CALCULATE.md`](../../MOBILE/ORDER/CALCULATE.md#response) buat contoh & penjelasan
(`dpp`/`net_dpp`/`tax_amount`/`total` per 1 unit, `sub_total`/`total_tax`/`total_billing` udah
di-scale qty, formula DPP-first). Karena gak ada promo: `promo_id` selalu `null`, `promo_name`
`null`, `discount_amount` `"0.00"`, `total_discount` `"0.00"`, `net_dpp == dpp`.

## Validasi

Semua validasi versi member app berlaku **kecuali** yang soal promo (14 barrier promo gak
dijalanin sama sekali) — daftar lengkapnya di
[`../MOBILE/ORDER/CALCULATE.md`](../../MOBILE/ORDER/CALCULATE.md#validasi):
`items tidak boleh kosong`, `qty item wajib lebih dari 0`, `item tidak ditemukan di menu
branch/visit purpose ini` (termasuk `qr_order = false`), `package tidak ditemukan buat item ini`,
`pilihan package tidak ditemukan di grup ini`, `qty pilihan package wajib lebih dari 0`, `jumlah
pilihan package di luar batas min/max grup`.

Ditambah: 4 kode identitas gak valid → error per langkah (tabel di
[`GET VISIT PURPOSE DETAIL.md`](./GET%20VISIT%20PURPOSE%20DETAIL.md#error)), dan `use_promo_ids`
terisi → `promo belum didukung di QR Order`.

## Catatan implementasi

`calculateOrder()` dipanggil apa adanya lewat `memberID=0` (bukan `nil`/opsi baru — sesuai
signature yang udah ada, `int64`) — aman karena `use_promo_ids` udah dipastikan kosong SEBELUM
`calculateOrder()` dipanggil, jadi cabang promo (`fetchMemberPromoContext`) gak pernah kesentuh.
Handler-nya (`order_qr_calculate_handler.go`) hidup di package `order` yang sama kayak Create,
pola identik (`qrCalculateRequest` mirip `qrCreateOrderRequest` minus identitas tamu+pembayaran).

## Tervalidasi live (2026-09-17)

Branch 51 (`SBE`)/company `SUDO`/visit purpose 7 (`ASD`), item `109` qty 2:
- Sukses → `sub_total: "223860.00"` (= `111930×2`), `total_billing: "246246.00"` (= `123123×2`) —
  konsisten sama harga satuan yang dipakai Create Order (qty 1 di situ).
- `db_code` kosong → `"db_code wajib diisi"`. `items: []` → `"items tidak boleh kosong"`.
  `use_promo_ids: [23]` → `"promo belum didukung di QR Order"` (ditolak, bukan diabaikan).
  `menu_id` gak ada di menu → `"item tidak ditemukan di menu branch/visit purpose ini"`.
  `company_code` salah → `"company tidak ditemukan"`.

Endpoint ini baca-only, gak ada data yang perlu dibersihin.
