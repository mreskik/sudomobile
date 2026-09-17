# Account - Point History

Riwayat transaksi poin member yang lagi login, terbaru duluan.

```
GET /api/account/point/history?start_date=2026-08-01&end_date=2026-08-21
Authorization: Bearer <token>
```

**Protected** — `member_id` dari session token, sama pola kayak [ME.md](ME.md).

## Parameter Query (opsional)

- `start_date` / `end_date` (format `YYYY-MM-DD`).
- **Dua-duanya kosong → default HARI INI** (bukan "gak difilter sama sekali") — sama aturan kayak [BALANCE HISTORY.md](BALANCE%20HISTORY.md), biar gak narik seluruh histori member tanpa sengaja.
- Filter berdasarkan `transaction_date`, **inklusif** `end_date`.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 8,
      "transaction_date": "2026-08-20T00:00:00Z",
      "transaction_type": "earn",
      "reference_number": "NO1TB20260820191830",
      "point_config_name": "Poin Kopi Weekend",
      "point_in": "50",
      "point_out": "0",
      "balance_after": "500",
      "notes": null
    }
  ]
}
```

| Field | Keterangan |
|---|---|
| `transaction_type` | `earn` (dapet poin dari order), `redeem` (poin ditukar), dst |
| `reference_number` | Referensi transaksi — buat `earn` dari order, ini `order_number`-nya |
| `point_config_name` | Nama config poin (`master_member_point_config`) yang ngasih poin ini. **`null` buat baris `redeem`** — redeem gak berasal dari config poin, jadi gak ada nama config-nya |
| `point_in`/`point_out` | Poin masuk/keluar transaksi ini |
| `balance_after` | Snapshot poin **setelah** transaksi ini |

List kosong `[]` kalau gak ada transaksi di rentang tanggal itu (bukan error).

## Catatan

- Sama persis query/aturan `GetPointHistory()` di `sudocore2` (`backend/modules/master/member/member_services.go`) — cuma di-scope ke member yang lagi login lewat session, bukan terima `member_id` dari param kayak endpoint ERP-nya.
- Baris yang `is_deleted = true` gak ikut muncul.
- Urutan **terbaru duluan** (`created_at DESC, id DESC`).

## Tervalidasi

**⚠️ Belum tervalidasi lewat HTTP request** (2026-08-21) — query tervalidasi sintaksnya lewat `psql` (jalan tanpa error), tapi `member_point_ledger` **kosong total** di seluruh sistem saat ini, jadi belum ada baris asli buat mastiin bentuk output. `go build`/`go vet` bersih. Update bagian ini kalau udah ada data & dites.
