# Account - Balance

Saldo **terkini** member yang lagi login.

```
GET /api/account/balance
Authorization: Bearer <token>
```

**Protected** — `member_id` dari session token, sama pola kayak [ME.md](ME.md).

## Response

```json
{ "code": 0, "message": "success", "data": { "balance": "150000.00" } }
```

- `balance` — string numeric, saldo terkini. **Belum pernah ada transaksi saldo sama sekali** (member baru, gak error) → `"0.00"`.

## Catatan

- **Bukan `SUM`** — saldo terkini itu `balance_after` di baris **TERAKHIR** `member_balance_ledger` (`ORDER BY created_at DESC, id DESC LIMIT 1`), sama persis formula `GetBalance()` di `sudocore2` (`backend/modules/master/member/member_services.go`). Tiap baris ledger udah nyimpen snapshot saldo setelah transaksi itu, jadi gak perlu jumlahin semua baris.
- Mau riwayat transaksi, bukan cuma angka terkini? Lihat [BALANCE HISTORY.md](BALANCE%20HISTORY.md).
- Cara top-up saldo belum ada dokumentasinya di sini (belum ada endpoint top-up dari `sudomobile` — kalau ada, jalurnya lewat Kiosk/`member_topup_online`, lihat `sudocore2`/`POS`).

## Tervalidasi

**⚠️ Belum tervalidasi lewat HTTP request** (2026-08-21) — query tervalidasi sintaksnya lewat `psql` (jalan tanpa error), tapi `member_balance_ledger` **kosong total** di seluruh sistem saat ini (belum ada transaksi topup sama sekali), jadi belum ada baris asli buat mastiin bentuk output. `go build`/`go vet` bersih. Update bagian ini kalau udah ada data & dites.
