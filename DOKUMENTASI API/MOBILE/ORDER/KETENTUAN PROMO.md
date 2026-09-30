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
| 0 | Maksimal 1 promo per order (2026-09-30) | `len(use_promo_ids)` | `"cuma boleh pakai maksimal 1 promo per order"` |
| 0b | Wajib member kalau `flag_required_member=true` (2026-09-30, REVISI urutan — dicek SEBELUM #1-8) | `master_promo.flag_required_member` (`pricing.CheckPromoRequiresMember()`, query ringan) vs ada/gaknya `member_id` (login) | `"This promo is for members only"` |
| 1 | `is_active = true` | `master_promo.is_active` | promo dianggap gak ketemu |
| 2 | Dalam periode berlaku | `period_start`/`period_end` vs tanggal sekarang | promo dianggap gak ketemu |
| 3 | Channel cocok | `flag_apply_to_all` ATAU ada baris `master_promo_apply_to` dengan `apply_to='mobile_customer'` | promo dianggap gak ketemu |
| 4 | Branch cocok | `flag_all_branches` ATAU ada baris `master_promo_branches` match `branch_id` request | promo dianggap gak ketemu |
| 5 | Visit purpose cocok | `flag_all_visit_purposes` ATAU ada baris `master_promo_visit_purposes` match `visit_purpose_id` request | promo dianggap gak ketemu |
| 6 | Tipe member cocok | `flag_all_type_members` ATAU ada baris `master_promo_type_members` match `member_type_id` customer yang login | promo dianggap gak ketemu |
| 6b | Tier member cocok (2026-09-30) | `flag_all_tiers` ATAU ada baris `master_promo_tier` match `tier_level` (`master_member.tier_level`) customer yang login | promo dianggap gak ketemu |
| 7 | Hari cocok | `flag_all_days` ATAU ada baris `master_promo_days` match hari ini | promo dianggap gak ketemu |
| 8 | Jam cocok | `flag_all_times` ATAU ada baris `master_promo_times` match jam sekarang | promo dianggap gak ketemu |
| 9 | Target cocok ke MINIMAL 1 baris item di cart | `promo_for` (category/subcategory/item) vs `master_promo_categories`/`_sub_categories`/`_items` | `"promo {id} tidak berlaku buat item apa pun di cart"` |
| 10 | Gak rebutan baris sama promo lain di request yang sama | — | `"promo {id1} dan {id2} sama-sama cocok ke item {nama} -- pilih salah satu"` (⚠️ secara praktik gak akan pernah kena lagi sejak barrier #0 ada — kodenya sengaja dibiarkan sebagai defensive check, bukan dihapus) |
| 11 | Subtotal belanja (sebelum diskon apa pun) capai `min_buy_amount` | `master_promo.min_buy_amount` | `"belanja belum mencapai minimum buat promo {id}"` |
| 12 | Saldo poin member capai `min_point_amount` | `master_promo.min_point_amount` vs `member_point_ledger` terbaru | `"poin member gak cukup buat promo {id}"` |
| 13 | Belum kepake `apply_limit_per_day` kali hari ini | dihitung dari `mb_order_detail` (lihat "Beda dari POS") | `"promo {id} udah mencapai limit pemakaian hari ini"` |
| 14 | Tipe promo bukan `freeitem` | `master_promo.type` | ditolak, `freeitem` belum didukung |

Barrier #1-8 (termasuk #6b) di-cek dalam SATU query (`pricing.ResolvePromo()`), mirror persis filter yang dipakai `MasterController::GetPromoList()` di POS (channel di-hardcode `mobile_customer`, bukan `pos`) DITAMBAH barrier tier (`master_promo_tier`). Barrier #0b dicek TERPISAH dan LEBIH DULU (query ringan sendiri), barrier #9-14 dicek terpisah setelahnya.

**Kenapa #0b dipisah dan didahulukan (2026-09-30, REVISI dari desain awal)**: awalnya `flag_required_member` dicek SETELAH `ResolvePromo()` (jadi setelah #1-8). Masalahnya: kalau guest (gak login) nyoba promo yang butuh member_type/tier tertentu, `ResolvePromo()` bakal balikin `nil` DULUAN (karena guest gak punya `member_type_id`/`tier_level` yang valid, defaultnya `0`, gak akan pernah match barrier #6/#6b) — pesannya jadi generik `"promo tidak ditemukan / tidak berlaku"`, padahal akar masalahnya lebih mendasar (emang gak login). Sekarang `flag_required_member` dicek LEBIH DULU lewat `pricing.CheckPromoRequiresMember()` (query ringan, cuma `flag_required_member` + `is_active`/periode — sengaja tetep ngecek 2 itu biar gak bocorin status "butuh member" buat promo yang udah expired/nonaktif) — guest yang nyoba promo khusus-member sekarang dapet pesan yang tepat sasaran duluan.

**Barrier #6b — `master_promo_tier`** (2026-09-30, migration sudocore2 `233_alter_table_master_promo_add_tier_barrier.sql`): promo bisa dibatasin cuma berlaku buat tier member tertentu (mis. "cuma tier 1 dan 2"), pola SAMA PERSIS kayak barrier tipe member (#6) — `master_promo.flag_all_tiers` (default `true`, semua tier boleh) + tabel child `master_promo_tier(promo_id, tier_level)`, bisa lebih dari 1 baris per promo. Kolomnya `tier_level` (BUKAN `tier_id`) — tier di sistem ini gak punya PK surrogate `id`, `level`-nya sendiri (`master_member_tier_setting_detail.level`, integer 1/2/3/dst) yang jadi identitas, sama kayak `master_member.tier_level` yang udah ada. `tier_level` member 0 (fallback, gak ketemu) gak akan pernah match baris manapun (level asli gak ada yang 0) — otomatis jatuh ke `flag_all_tiers=true` doang.

**Ditegakkan juga di POS lokal** (2026-09-30, REVISI dari kondisi awal — POS sebelumnya gak punya konsep tier member SAMA SEKALI):
- Skema baru `mr_member.tier_level` (`posv1-laravel`, migration `2026_09_30_041500_add_tier_to_mr_member_and_promo.php`) — disinkronkan dari `master_member.tier_level` ERP lewat `get_member_list` pull (`SetupServices::getMemberList()`), sama sekali gak diinput manual di POS. `master_member.tier_level` sendiri dihitung otomatis oleh job `membertierevaluation` (sudocore2, 1x/hari) — lihat memori project soal tier evaluation.
- `mr_promo.flag_all_tiers` + `mr_promo_tier` disinkronkan lewat endpoint baru `get_promo_tier` (mirror `get_promo_type_member`), 3 lapis: sudocore2 (`GetMasterPromoTier()`) → APIANDORDER (jembatan sync, `MasterService::GetMasterPromoTier()`) → POS (`SetupServices::getPromoTier()`).
- `MasterController::GetPromoList()` (POS) dapat query param baru `tier_level` (sejajar `member_type_id` yang udah ada, sama semantiknya — optional, `NULL` kalau member belum dipilih).
- `OrderServices::SaveOrder()` (POS) dapat validasi baru `ValidatePromoTierBarrier()` — kalau member udah dipilih TAPI tier-nya gak match salah satu `mr_promo_tier` promo yang dipakai (dan promo itu `flag_all_tiers=false`), order DITOLAK `"This promo is for members only"`. Beda dari barrier `flag_required_member` (yang nolak "gak ada member sama sekali") — ini nolak "ada member, tapi tier-nya gak sesuai".

**Barrier #12 (2026-09-30) — `min_point_amount` sekarang berperan GANDA**, bukan cuma barrier: nilainya sama persis yang dipakai buat **memotong** poin member kalau order beneran disubmit. Barrier ini yang di-cek di `Calculate()` (baca saldo doang, gak ada perubahan apa pun), tapi begitu order disubmit lewat `Create()`, poin sebesar `min_point_amount` ini **langsung dipotong** dari saldo member (insert `member_point_ledger`) — detail lengkap alur potong/refund-nya ada di [`CREATE ORDER.md`](CREATE%20ORDER.md#potong-poin-buat-promo-bersyarat-poin-min_point_amount-2026-09-30). Kalau lebih dari 1 promo di request sama-sama punya `min_point_amount > 0`, totalnya DIJUMLAH (bukan cuma yang terbesar/pertama).

**Barrier #0b — `flag_required_member`** (2026-09-30, migration sudocore2 `234_alter_table_master_promo_add_flag_required_member.sql`, REVISI 2x dari kelakuan lama):

1. **Revisi pertama**: sebelumnya ada guard BLOK TOTAL di `calculateOrder()` — `use_promo_ids` diisi TAPI `member_id == 0` (gak login) langsung ditolak `"promo tidak bisa dipakai tanpa login"`, SEBELUM promo apa pun di-resolve. Guard itu DIHAPUS, diganti validasi PER-PROMO lewat kolom baru `master_promo.flag_required_member` (default `false`) — niru kelakuan POS (`MasterController::GetPromoList()` PHP, `member_type_id` boleh `NULL`, promo yang gak butuh identitas member tetap bisa dipakai guest).
2. **Revisi kedua (urutan)**: dicek per-promo, tapi awalnya SETELAH `ResolvePromo()` (jadi setelah barrier #1-8) — dipindah jadi #0b, SEBELUM `ResolvePromo()`, lewat query ringan terpisah `pricing.CheckPromoRequiresMember()`. Lihat penjelasan "Kenapa #0b dipisah dan didahulukan" di atas.

- `flag_required_member = false` (default) → promo **tetap bisa dipakai tanpa login**, sama kayak POS.
- `flag_required_member = true` → promo WAJIB `member_id` ada, ditolak `"This promo is for members only"` kalau `member_id == 0`. Dicek per-promo SEBELUM `ResolvePromo()` (barrier #0b).
- **Divalidasi keras di sisi admin (sudocore2, `promo_services.go::validatePromoRequiredMember()`)** saat create/update promo: kalau promo punya barrier yang INHEREN butuh identitas member (`flag_all_type_members=false` ATAU `flag_all_tiers=false` ATAU `min_point_amount>0`) tapi `flag_required_member=false`, request DITOLAK `"This promo is for members only"` — kombinasi itu gak mungkin dievaluasi buat guest (gak ada `member_type_id`/`tier_level`/saldo poin buat guest).
- **Ditegakkan juga di POS lokal** (`OrderServices::SaveOrder()`, `posv1-laravel`) — beda dari POS sebelumnya yang gak punya validasi promo apa pun di backend (murni trust dari frontend Vue). Order yang `member_id` kosong tapi pakai promo `flag_required_member=true` (dicek dari `promo_id` tiap baris item/package) ditolak `Exception("This promo is for members only")`, SEBELUM item di-insert.
- Field ini juga dibalikin di [`GET List Promo`](KETENTUAN%20PROMO.md) (`promo_handler.go::GetList()`) sebagai info `flag_required_member`, biar FE bisa nampilin badge "khusus member" tanpa nebak.

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
