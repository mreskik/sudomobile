# Panduan Frontend — Menu & Order

Ringkasan alur bisnis lengkap: browsing menu sampai order kebayar. File detail per endpoint (request/response lengkap) ada di [`MENU/`](MENU) dan [`ORDER/`](ORDER) — dokumen ini isinya urutan pakainya + hal-hal yang gampang kesasar kalau cuma baca 1 file endpoint doang.

**Sebelum baca ini**, pastiin udah paham [`README.md`](README.md) (header wajib `X-App-Setting`/`Authorization`, format response `{code, message, data}`) — gak diulang di sini.

---

## 1. Alur lengkap (urutan normal)

```
GET /branch                                          → pilih lokasi
GET /branch/:id/visit-purpose                         → pilih visit purpose (takeaway/dine-in/dst)
GET /branch/:id/visit-purpose/:vp_id                   → tree menu (kategori→subkategori→item, harga, pajak, package)
GET /branch/:id/visit-purpose/:vp_id/best-seller       → (opsional) highlight item populer
GET /branch/:id/visit-purpose/:vp_id/promo             → (opsional, WAJIB LOGIN) daftar promo yang bisa dipilih
GET /branch/:id/visit-purpose/:vp_id/payment-method     → daftar metode bayar (gateway-only)
        │
        ▼  customer susun cart di sisi FE (client-side, gak ada endpoint "cart" di server)
        │
POST /order/calculate         (WAJIB LOGIN) → preview breakdown harga/pajak/promo, BACA-ONLY
        │
        ▼  customer klik "checkout"
        │
POST /order/create-order      (WAJIB LOGIN) → insert order + minta QR pembayaran, SEKALI JALAN
        │
        ▼  tampilin QR (vendor_qr_string/vendor_qr_url dari response create-order)
        │
GET /order/:order_number/payment-status   → POLLING tiap beberapa detik sambil QR ditampilin
   (atau)
GET /order/:order_number                  → buka detail order (breakdown + status + QR ulang kalau masih pending)
        │
        ├─ status jadi "paid"    → tampilin sukses/struk
        ├─ status jadi "expired" → tampilin "kadaluarsa, checkout ulang" (BUKAN retry, order baru)
        └─ customer batal duluan → POST /order/:order_number/cancel (cuma bisa kalau masih pending)

GET /order/history             (WAJIB LOGIN) → riwayat order milik customer yang login
```

**Endpoint publik** (gak butuh `Authorization`, cuma `X-App-Setting`): `GET /branch`, `.../visit-purpose`, `.../visit-purpose/:id`, `.../payment-method`, `.../best-seller`. **Endpoint protected** (wajib login): `.../promo`, semua `/order/*`.

---

## 2. Menyusun cart (PENTING — gak ada endpoint "cart" di server)

Server **gak nyimpen state cart apa pun**. Client yang nyusun body request `items[]` sendiri dari hasil `GET .../visit-purpose/:vp_id` (ambil `menu_id`, dan kalau ada package, `package_id`+`menu_package_id` dari `package_list` response itu). Body ini dipakai **sama persis** buat `Calculate` maupun `Create` — cuma beda endpoint:

```json
{
  "branch_id": 51,
  "visit_purpose_id": 7,
  "use_promo_ids": [23],
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

**Client CUMA kirim identitas + qty — TIDAK PERNAH kirim harga/pajak.** Server yang resolve semuanya dari DB tiap kali. Kalau FE nyimpen harga item di local state buat nampilin cart (wajar, biar gak nge-fetch ulang tiap render), **jangan pernah kirim balik angka itu ke server** — `Calculate`/`Create` bakal ngitung ulang dari nol dan itu yang jadi kebenaran, harga yang di-cache FE cuma buat display.

`payment_method_id` **cuma dibutuhin di `Create`**, gak ada di `Calculate` (preview gak butuh tau cara bayar).

## 3. `use_promo_ids` — client gak perlu tau matching-nya

Promo dikirim **level order** (`use_promo_ids: [23]`), **sejajar** `items`, **bukan** nested per-item. FE nunjukin daftar promo dari `GET .../promo` (biasanya lewat UI "pilih promo"), lalu customer pilih 1+ promo, id-nya masuk `use_promo_ids`. **Server yang nyari sendiri** baris item mana yang cocok jadi target tiap promo — FE gak perlu ngerti `promo_for`/target matching sama sekali.

Yang PERLU ditangani FE:
- `GET .../promo` balikin `min_buy_amount`/`min_point_amount`/`apply_limit_per_day`/`used_today` sebagai **info mentah, bukan filter** — promo yang syaratnya belum kepenuhi TETAP muncul di list. FE yang nentuin mau nampilin promo itu disabled/greyed-out berdasar info itu, atau biarin aja customer coba pilih dan biarin `Calculate`/`Create` yang nolak dengan pesan jelas.
- Kalau `Calculate`/`Create` balikin error terkait promo (lihat [`KETENTUAN PROMO.md`](ORDER/KETENTUAN%20PROMO.md) buat daftar lengkap pesannya), **itu bukan bug** — tampilin pesannya apa adanya ke customer (misal "promo X dan Y sama-sama cocok ke item Z -- pilih salah satu" kalau customer pilih 2 promo yang rebutan 1 item).

## 4. Angka di response `Calculate`/`Create` — per-unit vs total

**Gampang salah kalau gak diperhatiin**: field di level `items[]` (`dpp`/`net_dpp`/`tax_amount`/`total`) itu **PER 1 UNIT** (belum dikali `qty`). Field di level ATAS (`sub_total`/`total_tax`/`total_discount`/`total_billing`) itu **udah di-scale**. Jangan kali `qty` manual ke field level atas (udah kekalikan), tapi WAJIB kali `qty` kalau mau nampilin subtotal per baris item dari field `items[].total`.

Package/sub-item malah **dikali qty item induk DIKALI qty selection-nya** — misal item qty 2, tiap porsi ambil 1 varian, varian itu ke-scale ×2 juga di total level atas (bukan ×1).

## 5. Status order (`mb_order.status`) — state machine

```
pending ──(bayar sukses)──────────→ paid
   │
   ├──(QR expired, gak dibayar)───→ expired
   │
   └──(customer cancel manual)────→ cancel
```

- `pending` → `paid`/`expired` **gak otomatis real-time** — butuh ada yang manggil `payment-status` atau buka `order-detail` (client-triggered), ATAU nunggu [background job `orderexpiry`](../DOKUMENTASI%20BACKGROUND%20JOB/ORDER%20EXPIRY.md) jalan (tiap 5 menit, jaring pengaman doang — **jangan andelin ini buat UX real-time**, FE tetap harus polling `payment-status` sendiri sambil QR ditampilin).
- **Gak ada state kelima.** Order yang `expired`/`cancel` **gak bisa "dilanjutkan"** — belum ada endpoint retry, satu-satunya jalan customer checkout lagi adalah bikin order BARU (`create-order` lagi dari cart). Jangan desain UI yang nawarin "bayar lagi" ke order yang udah `expired`/`cancel` — arahin ke "checkout ulang" (balik ke cart, bukan ke order lama).
- Kalau customer nutup app/kehilangan koneksi PAS QR lagi ditampilin dan buka lagi nanti (order masih `pending`), **QR yang sama bisa dimunculin ulang** lewat `GET /order/:order_number` (`order-detail`) — field `payment.vendor_qr_string`/`vendor_qr_url` bakal keisi lagi kalau statusnya masih `pending`, `null` kalau udah bukan `pending`. Ini BUKAN minta QR baru, jadi kode/nominal-nya tetap sama.

## 6. Ringkasan Endpoint

### Menu — publik, base `GET /api/`

| Path | Fungsi |
|---|---|
| `branch` | Daftar branch yang nerima order mobile |
| `branch/:branch_id/visit-purpose` | Daftar visit purpose per branch |
| `branch/:branch_id/visit-purpose/:visit_purpose_id` | Tree menu + harga + pajak + package |
| `branch/:branch_id/visit-purpose/:visit_purpose_id/payment-method` | Daftar metode bayar (gateway-only) |
| `menu/best-seller` | Best seller global (30 hari, tanpa harga) |
| `branch/:branch_id/best-seller` | Best seller per branch (tanpa harga) |
| `branch/:branch_id/visit-purpose/:visit_purpose_id/best-seller` | Best seller + harga |

### Order — protected, base `/api/order` kecuali disebut lain

| Method | Path | Fungsi |
|---|---|---|
| GET | `branch/:branch_id/visit-purpose/:visit_purpose_id/promo` | Daftar promo eligible |
| POST | `order/calculate` | Preview breakdown, baca-only |
| POST | `order/create-order` | Submit order + trigger QR pembayaran |
| GET | `order/:order_number/payment-status` | Polling status bayar |
| GET | `order/:order_number` | Detail order + QR ulang |
| POST | `order/:order_number/cancel` | Batal (cuma kalau masih pending) |
| GET | `order/history` | Riwayat order (header doang) |

## 7. Keterbatasan yang WAJIB diketahui FE (biar gak salah desain UX)

- **Order yang gagal bayar TIDAK bisa di-retry** — belum ada endpoint-nya (keputusan sadar, bukan belum sempat). Solusinya: checkout ulang dari cart (order_number baru).
- **Promo tipe `freeitem` gak didukung** — kalau ada promo begini di data, jangan tampilin di UI pemilihan promo (atau tampilin tapi disabled), karena kalau dipaksa dipakai bakal ditolak server.
- **`apply_limit_per_item` (cap qty yang boleh didiskon per baris) gak ditegakkan** — kalau promo match, SEMUA qty di baris itu kena diskon, bukan cuma sebagian. Jangan asumsikan ada partial-discount di 1 baris.
- **Order yang udah `paid` BELUM otomatis nyampe ke dapur/kasir POS** — mekanisme "pull" dari `mb_order` ke sistem POS lokal belum dibangun. Kalau ada requirement "customer liat status 'sedang disiapkan'/'siap diambil'", itu BELUM ada di API sekarang (cuma ada `pending`/`paid`/`cancel`/`expired`, bukan status dapur).
- **`order/history` baru header** — buat breakdown item per baris histori, panggil `GET /order/:order_number` (order-detail) pakai `order_number` dari hasil history.
- Semua endpoint `/order/*` **otomatis scoped ke member yang login** — gak ada cara akses order milik customer lain lewat endpoint yang sama (dan pesan errornya sengaja disamain antara "gak ketemu" vs "bukan punya kamu", jangan coba bedain di UI).
