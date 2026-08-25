# Branch - Get Payment Method List

```
GET /api/branch/:branch_id/visit-purpose/:visit_purpose_id/payment-method
```

**Publik** (gak butuh `Authorization`) — daftar payment method yang bisa dipakai buat 1 kombinasi branch+visit purpose. Nested di bawah [`GET VISIT PURPOSE DETAIL.md`](GET%20VISIT%20PURPOSE%20DETAIL.md) karena scoping-nya sama persis (branch + visit purpose).

## Request

`:branch_id` — id branch (dari `GET /api/branch`). `:visit_purpose_id` — FK ke `master_visit_purpose.id` (dari [`GET VISIT PURPOSE LIST.md`](GET%20VISIT%20PURPOSE%20LIST.md)). Gak ada body.

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

`branch_id`/`visit_purpose_id` yang bukan angka → `{ "code": 100, "message": "branch_id tidak valid" }` / `"visit_purpose_id tidak valid"`. Kombinasi yang gak punya payment method cocok → array kosong `[]`, bukan error.

## Analisa & keputusan desain (2026-08-24)

Sebelum dibuat, ditemukan 2 preseden yang beda filosofi di POS buat fitur serupa:

- **`KioskController::GetPaymentMethodList()`** — simpel, cuma filter `payment_gateway_code` gak kosong (Kiosk self-service, gak ada kasir yang bisa mungutin cash/manual). **Gak filter branch/visit_purpose sama sekali.**
- **`MasterController::GetPaymentMethod({visit_purpose_id})`** — filter visit_purpose lewat `JOIN` doang ke tabel junction. **Ketauan gak lengkap**: gak nangani `flag_all_visitpurpose=true` (payment method yang seharusnya berlaku ke SEMUA visit purpose tanpa perlu baris junction) — kemungkinan gap yang emang ada di POS sendiri, bukan sesuatu yang mau ditiru di sini (mirip kasus `service_charge` yang ketemu pas Tahap 2 [`GET VISIT PURPOSE DETAIL.md`](GET%20VISIT%20PURPOSE%20DETAIL.md) — "ada kolomnya tapi gak pernah dipakai bener").

**Keputusan (disepakati eksplisit lewat AskUserQuestion)**: filter gateway-only (samain Kiosk — mobile customer app itu online-order, gak ada kasir), DITAMBAH scoping branch+visit_purpose yang BENER (hormatin `flag_all_branch`/`flag_all_visitpurpose`), karena `sudomobile` ngelayanin banyak branch sekaligus (beda dari POS yang selalu 1 branch per install).

## Sumber data

```sql
SELECT DISTINCT mpm.id, mpm.name, mpm.code, mpm.color_theme
FROM master_payment_method mpm
WHERE mpm.is_active = true AND COALESCE(mpm.is_deleted, false) = false
	AND mpm.payment_gateway_code IS NOT NULL AND mpm.payment_gateway_code != ''
	AND (
		mpm.flag_all_branch = true
		OR EXISTS (
			SELECT 1 FROM master_payment_method_branches b
			WHERE b.payment_method_id = mpm.id AND b.branch_id = ?
				AND COALESCE(b.is_deleted, false) = false AND b.is_active = true
		)
	)
	AND (
		mpm.flag_all_visitpurpose = true
		OR EXISTS (
			SELECT 1 FROM master_payment_method_visit_purposes vp
			WHERE vp.payment_method_id = mpm.id AND vp.visitpurpose_id = ?
				AND COALESCE(vp.is_deleted, false) = false
		)
	)
ORDER BY mpm.name ASC
```

Baca langsung dari DB `sudocore2` (`sudomobile` connect ke DB yang sama, gak ada sync/bridge layer kayak POS↔APIANDORDER). Nama tabel junction visit-purpose sengaja dicatat karena gampang salah tebak: **`master_payment_method_visit_purposes`** (plural), bukan `master_payment_method_visit_purpose`.

`COALESCE(is_deleted, false)` dipakai di kedua tabel junction karena kolomnya nullable (pola yang sama kayak `master_pricelist_detail.is_deleted` yang ketemu bug-nya di [`GET VISIT PURPOSE DETAIL.md`](GET%20VISIT%20PURPOSE%20DETAIL.md)) — belum diverifikasi eksplisit bug-nya di tabel ini (gak ada data NULL di dev DB buat dites), tapi dipasang preventif karena polanya identik.

## Tervalidasi live (2026-08-24)

Dites ke data real: cuma 1 payment method di dev DB yang punya `payment_gateway_code` (QRIS, `flag_all_branch=true`, `flag_all_visitpurpose=false`). Baseline test `branch_id=51`/`visit_purpose_id=7` (fixture yang sama dipakai di endpoint menu) → `[]` (benar, junction visit-purpose QRIS cuma ke `visitpurpose_id=1`, bukan `7`). Ditambahin sementara baris junction `(payment_method_id=1, visitpurpose_id=7)` → QRIS langsung muncul, membuktikan resolusi `flag_all_branch=true` (branch manapun lolos) + junction visit-purpose bekerja bareng. Baris test dihapus lagi setelahnya (gak ada data pollution tersisa). Juga dites `branch_id`/`visit_purpose_id` non-angka → pesan error yang sesuai.

Belum sempat dites live: payment method dengan `flag_all_branch=false` (perlu baris di `master_payment_method_branches`) dan `flag_all_visitpurpose=true` — gak ada data existing buat itu di dev DB, tapi logic query-nya simetris sama yang udah kebukti jalan buat sisi visit_purpose.
