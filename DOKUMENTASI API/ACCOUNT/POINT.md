# Account - Point

Poin **terkini** member yang lagi login.

```
GET /api/account/point
Authorization: Bearer <token>
```

**Protected** — `member_id` dari session token, sama pola kayak [ME.md](ME.md).

## Response

```json
{ "code": 0, "message": "success", "data": { "point": "500" } }
```

- `point` — string numeric, poin terkini. **Belum pernah ada histori poin sama sekali** (member baru, gak error) → `"0"`.

## Catatan

- **Bukan `SUM`** — poin terkini itu `balance_after` di baris **TERAKHIR** `member_point_ledger` (`ORDER BY created_at DESC, id DESC LIMIT 1`), sama persis formula `GetPoint()` di `sudocore2`. Sama pola kayak [BALANCE.md](BALANCE.md), cuma sumber tabelnya beda.
- Poin didapat otomatis lewat background job [`pointcheck`](../../../sudocore2/DOKUMENTASI%20BACKGROUND%20JOB/POINTCHECK.md) (`sudocore2`) — evaluasi tiap 5 menit dari order `paid` yang match `master_member_point_config`, gak ada aksi manual dari customer buat dapetin poin.
- Mau riwayat transaksi, bukan cuma angka terkini? Lihat [POINT HISTORY.md](POINT%20HISTORY.md).

## Tervalidasi

**⚠️ Belum tervalidasi lewat HTTP request** (2026-08-21) — query tervalidasi sintaksnya lewat `psql` (jalan tanpa error), tapi `member_point_ledger` **kosong total** di seluruh sistem saat ini, jadi belum ada baris asli buat mastiin bentuk output. `go build`/`go vet` bersih. Update bagian ini kalau udah ada data & dites.
