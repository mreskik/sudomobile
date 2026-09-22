# Account - Top Up Check Status

Cek status percobaan top-up saldo dompet member yang lagi login. **PROTECTED**, dipanggil buat **POLLING** (mis. tiap beberapa detik) sambil QR ditampilin ke customer, abis manggil [`CREATE.md`](CREATE.md). Mirror alur [`PAYMENT STATUS.md`](../../ORDER/PAYMENT%20STATUS.md) order, plus **cek kepemilikan** (`member_topup_online.member_id` harus cocok member yang login) — top-up SELALU protected (beda dari order yang sekarang publik via `order_number`), jadi guard ini tetap ada.

```
GET /api/account/balance/topup/:reference_number/status
Authorization: Bearer <token>
```

## Response

```json
{ "code": 0, "message": "success", "data": { "reference_number": "TUSBE2026092210004571", "status": "paid", "balance_after": "250000.00" } }
```

`status` — `pending` / `paid` / `cancel` / `failed` / `expired` (di-remap dari `settlement` gateway ke `paid`, sama pola `PAYMENT STATUS.md` order). `balance_after` cuma keisi kalau `status == "paid"` — saldo TERKINI member setelah top-up ini (angka yang sama juga dibalikin [`BALANCE.md`](../BALANCE.md)).

Topup gak ketemu / bukan punya member yang login (pesan disamain, gak bocorin kepemilikan) → `{ "code": 100, "message": "topup tidak ditemukan" }`.

## Alur di dalamnya

1. **Cek kepemilikan** — `member_topup_online.member_id` harus cocok member yang login.
2. **Idempotency guard** — kalau `status` udah `paid`, langsung balikin `paid` + saldo terkini **tanpa** ngecek ulang ke gateway atau insert `member_balance_ledger` lagi.
3. Kalau belum, live-check `GET {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{reference_number}`.
4. **Fallback expired** — `expired_at` lokal udah lewat tapi gateway masih bilang `pending` (webhook mungkin gak akan pernah nyampe) → cancel eksplisit ke gateway, treat `expired`.
5. Kalau `settlement` → **insert `member_balance_ledger`** (`transaction_type = 'topup'`, `source = 'mobile'`, `balance_in = amount`, `balance_after` dihitung di SQL: saldo lama + `amount`, row `master_member` di-lock `FOR UPDATE` dulu biar gak race sama transaksi saldo lain buat member yang sama) DALAM 1 TRANSAKSI dengan update `member_topup_online.status = 'paid'`. Guard `WHERE status = 'pending'` di update (race 2 polling bersamaan) — kalau `RowsAffected() = 0`, udah kepake proses lain duluan, ambil saldo terkini aja tanpa insert ledger dobel.
6. Kalau `expired`/`cancel`/`failed` → `member_topup_online.status` disinkronin (guard `WHERE status = 'pending'`).

## Jaring pengaman (2026-09-22)

Kalau customer nutup app sebelum sempat polling sampai selesai, background job [`topupstatuschanger`](../../../../DOKUMENTASI%20BACKGROUND%20JOB/TOPUP%20STATUS%20CHANGER.md) (jalan tiap 1 menit) otomatis manggil ulang alur di atas buat top-up yang `expired_at`-nya udah lewat tapi gak pernah dipoll lagi — mencegah baris `pending` nyangkut selamanya.

## Sumber data / implementasi

- `sudomobile/backend/modules/topup/topup_service.go` — `CheckTopupStatus()`, `confirmTopupPaid()`, `lockMemberAndInsertLedger()`, `getLastBalance()`.
- `sudomobile/backend/modules/topup/topup_handler.go` — `CheckStatus()`.
- `sudomobile/backend/modules/topupstatuschanger/` — background job jaring pengaman, lihat [`TOPUP STATUS CHANGER.md`](../../../../DOKUMENTASI%20BACKGROUND%20JOB/TOPUP%20STATUS%20CHANGER.md).
- Referensi/pola: `APIANDORDER/backend/modules/apipos/membertopup/membertopup_service.go` (`confirmTopupPaid()`, `lockMemberAndInsertLedger()` — logic ledger identik, cuma `source` beda: `'mobile'` di sini vs bisa `'pos'`/`'kiosk'` di situ).

## Status

**Baru dibuat (2026-09-22), belum tervalidasi live** — `go build`/`go vet` bersih, belum dites end-to-end lewat HTTP request beneran (perlu service `payment` jalan + member session token real). Update bagian ini setelah dites.
