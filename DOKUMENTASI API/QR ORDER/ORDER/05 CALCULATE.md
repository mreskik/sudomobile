# QR Order - Calculate

**Status: SELESAI & tervalidasi live (2026-09-17). Promo publik dibuka (2026-09-30, REVISI).**

```
POST /qr-order/calculate?db_code=SUDO&company_code=SUDO&branch_code=SBJ&visit_purpose_code=DIN
Content-Type: application/json
```

**Publik** — tanpa `Authorization`, tanpa `X-App-Setting`. Preview breakdown harga/pajak isi
keranjang **sebelum** order disubmit, baca-only, gak insert apa pun. Versi QR Order dari
[`../MOBILE/ORDER/CALCULATE.md`](../../MOBILE/ORDER/CALCULATE.md) — body **sama persis** kayak
[`CREATE ORDER.md`](./06%20CREATE%20ORDER.md) minus field pembayaran/identitas, dan logic hitungnya
**fungsi yang sama** (`calculateOrder()`/`pricing.CalculateLine()`, DPP-first) — preview di
keranjang gak pernah beda sama yang beneran kesimpen.

**⚠️ REVISI 2026-09-30 — promo publik SEKARANG DIDUKUNG** (sebelumnya `use_promo_ids` ditolak
total, "belum didukung di QR Order"). QR Order tetep gak ada login (tamu murni, gak ada
`member_id`), jadi promo yang bisa dipakai OTOMATIS terbatas ke yang **gak butuh identitas
member sama sekali**: `flag_required_member=false` DAN `flag_all_type_members=true` DAN
`flag_all_tiers=true` — lihat [`04.1 GET LIST PROMO.md`](./04.1%20GET%20LIST%20PROMO.md) buat cara
nampilin promo publik yang eligible, dan [`KETENTUAN PROMO.md`](../../MOBILE/ORDER/KETENTUAN%20PROMO.md)
buat mekanisme lengkapnya (barrier yang sama persis dipakai di sini, bukan versi beda).

## Request

4 kode identitas wajib di **query param** (keputusan 2026-09-17: seragam buat semua endpoint QR
Order, `POST` sekalipun — body cuma isi cart), aturan & urutan cek sama persis
[`GET VISIT PURPOSE DETAIL.md`](./03%20GET%20VISIT%20PURPOSE%20DETAIL.md#request).

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
  diambil dari [`GET VISIT PURPOSE DETAIL.md`](./03%20GET%20VISIT%20PURPOSE%20DETAIL.md)).
- **`use_promo_ids`** (2026-09-30, REVISI) — array, **maksimal 1 elemen** (sama barrier #0
  [`KETENTUAN PROMO.md`](../../MOBILE/ORDER/KETENTUAN%20PROMO.md)). Diterusin APA ADANYA ke
  `calculateOrder()`, sama fungsi yang dipakai member app — TIDAK ADA validasi/logic terpisah
  khusus QR Order. Promo yang butuh identitas member (`flag_required_member=true`, ATAU
  `flag_all_type_members=false`, ATAU `flag_all_tiers=false`) OTOMATIS ketolak (QR Order gak
  pernah punya `member_id`) — lihat "Catatan implementasi" di bawah.

## Response

Sama persis versi member app — lihat
[`../MOBILE/ORDER/CALCULATE.md`](../../MOBILE/ORDER/CALCULATE.md#response) buat contoh & penjelasan
(`dpp`/`net_dpp`/`tax_amount`/`total` per 1 unit, `sub_total`/`total_tax`/`total_billing` udah
di-scale qty, formula DPP-first). Tanpa `use_promo_ids`: `promo_id` selalu `null`, `promo_name`
`null`, `discount_amount` `"0.00"`, `total_discount` `"0.00"`, `net_dpp == dpp`. Dengan promo
publik yang lolos, field-field itu keisi sama persis pola member app.

## Validasi

Semua validasi versi member app berlaku, TERMASUK barrier promo (lihat daftar lengkap di
[`KETENTUAN PROMO.md`](../../MOBILE/ORDER/KETENTUAN%20PROMO.md)) — daftar non-promo:
`items tidak boleh kosong`, `qty item wajib lebih dari 0`, `item tidak ditemukan di menu
branch/visit purpose ini` (termasuk `qr_order = false`), `package tidak ditemukan buat item ini`,
`pilihan package tidak ditemukan di grup ini`, `qty pilihan package wajib lebih dari 0`, `jumlah
pilihan package di luar batas min/max grup`.

Ditambah: 4 kode identitas gak valid → error per langkah (tabel di
[`GET VISIT PURPOSE DETAIL.md`](./03%20GET%20VISIT%20PURPOSE%20DETAIL.md#error)).

## Catatan implementasi

`calculateOrder()` dipanggil dengan `memberID=0` SELALU (QR Order = tamu murni, gak pernah ada
login) — `use_promo_ids` diterusin apa adanya lewat `calculateRequest.UsePromoIDs`, TANPA
validasi tambahan di handler QR (`order_qr_calculate_handler.go`). Karena `memberID=0`:
`fetchMemberPromoContext()` balikin `member_type_id=0`/`tier_level=0`/`memberPoint=0` (aman,
gak error) — barrier `flag_all_type_members`/`flag_all_tiers` di `ResolvePromo()` OTOMATIS gak
akan match promo yang dibatasin ke tipe/tier tertentu (`WHERE type_member_id = 0`/`tier_level = 0`
gak akan pernah ketemu baris asli), dan barrier `flag_required_member` (dicek LEBIH DULU, lihat
[`KETENTUAN PROMO.md`](../../MOBILE/ORDER/KETENTUAN%20PROMO.md) barrier #0b) langsung nolak kalau
`true`. Hasilnya: promo yang lolos ke QR Order SELALU promo publik murni, gak butuh whitelist
terpisah/logic khusus QR Order.

Handler-nya (`order_qr_calculate_handler.go`) hidup di package `order` yang sama kayak Create,
pola identik (`qrCalculateRequest` mirip `qrCreateOrderRequest` minus identitas tamu+pembayaran).

## Tervalidasi live (2026-09-17, promo belum ada saat itu)

Branch 51 (`SBE`)/company `SUDO`/visit purpose 7 (`ASD`), item `109` qty 2:
- Sukses → `sub_total: "223860.00"` (= `111930×2`), `total_billing: "246246.00"` (= `123123×2`) —
  konsisten sama harga satuan yang dipakai Create Order (qty 1 di situ).
- `db_code` kosong → `"db_code wajib diisi"`. `items: []` → `"items tidak boleh kosong"`.
  `menu_id` gak ada di menu → `"item tidak ditemukan di menu branch/visit purpose ini"`.
  `company_code` salah → `"company tidak ditemukan"`.

Endpoint ini baca-only, gak ada data yang perlu dibersihin.

**Belum tervalidasi live buat revisi promo 2026-09-30** — logic-nya reuse 100% `calculateOrder()`
yang udah tervalidasi di member app (`CALCULATE.md`), tapi kombinasi spesifik "QR Order + promo
publik" belum dites end-to-end. Next step kalau butuh validasi tambahan.
