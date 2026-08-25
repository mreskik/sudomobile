# Order - Ketentuan Promo

Dokumen ini ngejelasin MEKANISME promo secara menyeluruh — buat spek request/response endpoint yang beneran makai ini, lihat [`CALCULATE.md`](CALCULATE.md) (bagian "Promo"). Data promo (`master_promo`) hidup di ERP (`sudocore2`), dipakai bareng sama POS — tapi cara `sudomobile` MEMPROSES-nya beda total dari POS, lihat bagian "Beda dari POS" di bawah.

## Alur singkat

1. Client kirim `use_promo_ids` (array id `master_promo`) di level ORDER, sejajar `items` — **bukan** nested per-item. Client cuma bilang "aku mau pakai promo ini", **server yang nyari sendiri** baris item mana di cart yang cocok jadi targetnya.
2. Server resolve tiap promo lewat serangkaian filter eligibility (lihat bawah). Promo yang gak lolos → REQUEST DITOLAK (bukan di-skip diam-diam).
3. Server cari baris item di cart yang cocok target promo itu (`promo_for`). Kalau ketemu lebih dari 1 baris yang cocok, SEMUA baris itu kena diskon. Kalau gak ada satu pun yang cocok → ditolak.
4. Diskon dihitung per tipe promo (`rupiah`/`percent`), diterapkan ke `dpp` baris item itu, ngikutin formula DPP-first yang sama kayak perhitungan pajak (lihat [`CALCULATE.md`](CALCULATE.md#formula)).

## Barrier / validasi (lengkap)

Setiap promo yang diminta HARUS lolos SEMUA ini, atau request ditolak total (gak ada partial-apply):

| # | Barrier | Sumber | Kalau gagal |
|---|---|---|---|
| 1 | `is_active = true` | `master_promo.is_active` | promo dianggap gak ketemu |
| 2 | Dalam periode berlaku | `period_start`/`period_end` vs tanggal sekarang | promo dianggap gak ketemu |
| 3 | Channel cocok | `flag_apply_to_all` ATAU ada baris `master_promo_apply_to` dengan `apply_to='mobile_customer'` | promo dianggap gak ketemu |
| 4 | Branch cocok | `flag_all_branches` ATAU ada baris `master_promo_branches` match `branch_id` request | promo dianggap gak ketemu |
| 5 | Visit purpose cocok | `flag_all_visit_purposes` ATAU ada baris `master_promo_visit_purposes` match `visit_purpose_id` request | promo dianggap gak ketemu |
| 6 | Tipe member cocok | `flag_all_type_members` ATAU ada baris `master_promo_type_members` match `member_type_id` customer yang login | promo dianggap gak ketemu |
| 7 | Hari cocok | `flag_all_days` ATAU ada baris `master_promo_days` match hari ini | promo dianggap gak ketemu |
| 8 | Jam cocok | `flag_all_times` ATAU ada baris `master_promo_times` match jam sekarang | promo dianggap gak ketemu |
| 9 | Target cocok ke MINIMAL 1 baris item di cart | `promo_for` (category/subcategory/item) vs `master_promo_categories`/`_sub_categories`/`_items` | `"promo {id} tidak berlaku buat item apa pun di cart"` |
| 10 | Gak rebutan baris sama promo lain di request yang sama | — | `"promo {id1} dan {id2} sama-sama cocok ke item {nama} -- pilih salah satu"` |
| 11 | Subtotal belanja (sebelum diskon apa pun) capai `min_buy_amount` | `master_promo.min_buy_amount` | `"belanja belum mencapai minimum buat promo {id}"` |
| 12 | Saldo poin member capai `min_point_amount` | `master_promo.min_point_amount` vs `member_point_ledger` terbaru | `"poin member gak cukup buat promo {id}"` |
| 13 | Belum kepake `apply_limit_per_day` kali hari ini | dihitung dari `mb_order_detail` (lihat "Beda dari POS") | `"promo {id} udah mencapai limit pemakaian hari ini"` |
| 14 | Tipe promo bukan `freeitem` | `master_promo.type` | ditolak, `freeitem` belum didukung |

Barrier #1-8 di-cek dalam SATU query (`pricing.ResolvePromo()`), mirror persis filter yang dipakai `MasterController::GetPromoList()` di POS (channel di-hardcode `mobile_customer`, bukan `pos`). Barrier #9-14 dicek terpisah setelahnya.

## Tipe promo & formula diskon

| Tipe | Formula | Status |
|---|---|---|
| `rupiah` | `discount_amount = type_rupiah_amount` (flat, per unit) | ✅ didukung |
| `percent` | `discount_amount = dpp × type_percent_rate / 100`, di-cap `type_percent_limit_amount` kalau `type_percent_use_limit = true` | ✅ didukung |
| `freeitem` | nambah baris item gratis baru ke cart (bukan diskon di baris existing) | ❌ belum didukung, ditolak eksplisit |

`discount_amount` hasil hitungan DICLAMP maksimal sebesar `dpp` item itu sendiri (gak mungkin bikin harga jadi negatif) — ini logic pengaman BARU yang ditambahin di `sudomobile` (POS gak punya ini karena POS emang gak pernah ngitung diskon di backend sama sekali, lihat bagian bawah).

**`apply_limit_per_item`** (cap qty per item yang boleh didiskon) **belum ditegakkan** — kalau promo match 1 baris qty 5, diskon diterapkan rata ke SEMUA qty 5, bukan cuma sebagian. Ini keterbatasan MVP yang didokumentasikan sadar, bukan kelupaan.

## Multi-match & konflik

- **1 promo bisa kena ke lebih dari 1 baris item** — kalau target-nya `category`/`subcategory` dan cart punya beberapa item dari kategori itu, SEMUA baris itu dapet diskon (bukan cuma 1).
- **2 promo yang sama-sama cocok ke baris yang SAMA DITOLAK** — skema (`mb_order_detail.promo_id`) cuma muat 1 promo per baris, gak ada stacking. Server gak nebak salah satu duluan (first-match-wins) — request ditolak total, biar client/customer yang mutusin mau pakai promo yang mana.
- **Promo yang gak match apa pun di cart DITOLAK**, bukan diabaikan — kalau customer eksplisit minta promo tapi gak kena ke mana-mana, itu dianggap kesalahan (salah pilih promo/item) yang perlu dikasih tau, bukan situasi normal yang di-silent.

## Beda dari POS (penting)

POS (`OrderServices.php`) **gak punya validasi/kalkulasi promo di backend sama sekali** — `promo_id`/`discount_percent`/`discount_amount` yang dikirim dari frontend Kiosk/kasir LANGSUNG disimpen mentah-mentah, backend cuma percaya. Eligibility filtering di POS (`MasterController::GetPromoList()`) cuma buat NAMPILIN daftar promo yang bisa dipilih — matching & hitung diskonnya tetep di frontend.

`sudomobile` **SENGAJA gak niru pola ini** — konsisten sama prinsip "server gak pernah percaya harga/diskon dari client" yang dipegang di seluruh endpoint order (`Calculate`). Server yang resolve eligibility DAN hitung `discount_amount` sendiri dari nol.

## Scope pemakaian promo terpisah dari POS

`apply_limit_per_day` di-hitung dari `mb_order_detail` **doang** (order mobile customer) — SENGAJA gak digabung sama pemakaian promo yang sama di sisi POS (`pos_order_detail`), walau `promo_id`-nya entity yang sama di `master_promo`. Ini keputusan scope yang disepakati eksplisit (2026-08-24): **POS gak perlu ngitung ulang atau tau apa pun soal order dari mobile**. Master data promo boleh dipakai bareng, tapi limit/kalkulasi pemakaian masing-masing channel independen.

## Implementasi

- `sudomobile/backend/pricing/promo.go` — `ResolvePromo()`, `PromoTargetMatches()`, `PromoUsedToday()`, `CalculatePromoDiscount()`.
- `sudomobile/backend/modules/order/order_handler.go` — resolusi & matching `use_promo_ids` (fungsi `calculateOrder()`, blok sebelum PASS 2).
- Skema: `master_promo` + 8 tabel anak (`_apply_to`/`_branches`/`_categories`/`_days`/`_items`/`_sub_categories`/`_times`/`_type_members`) di `sudocore2`. `mb_order_detail.promo_id`/`discount_percent`/`discount_amount` (belum ada tabel snapshot terpisah — disepakati cukup gitu, 2026-08-25).

## Status

Berlaku buat `POST /api/order/calculate` (preview) dan `GET .../promo` ([`GET LIST PROMO.md`](GET%20LIST%20PROMO.md), buat nampilin daftar promo eligible sebelum dipilih). Logic yang sama dipakai ulang persis di [`POST /api/order/create-order`](CREATE%20ORDER.md) (save order beneran).
