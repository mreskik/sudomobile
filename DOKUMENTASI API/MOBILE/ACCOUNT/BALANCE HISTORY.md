# Account - Balance History

Riwayat transaksi saldo member yang lagi login, terbaru duluan.

```
GET /api/account/balance/history?start_date=2026-08-01&end_date=2026-08-21
Authorization: Bearer <token>
```

**Protected** — `member_id` dari session token, sama pola kayak [ME.md](ME.md).

## Parameter Query (opsional)

- `start_date` / `end_date` (format `YYYY-MM-DD`).
- **Dua-duanya kosong → default HARI INI** (bukan "gak difilter sama sekali") — sengaja gitu, biar gak narik seluruh histori member tanpa sengaja kalau frontend lupa kirim filter.
- Filter berdasarkan `transaction_date`, **inklusif** `end_date` (sampai akhir hari itu, bukan cuma jam `00:00:00`).

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "id": 12,
      "transaction_date": "2026-08-20T00:00:00Z",
      "transaction_type": "topup",
      "source": "online",
      "reference_number": "TOPUP20260820001",
      "balance_in": "100000.00",
      "balance_out": "0.00",
      "balance_after": "150000.00",
      "notes": null
    }
  ]
}
```

| Field | Keterangan |
|---|---|
| `transaction_type` | `topup`, `payment` (saldo dipakai bayar order), `refund`, `adjustment` |
| `source` | Asal transaksi (mis. `online`, `erp`) |
| `balance_in`/`balance_out` | Nominal masuk/keluar transaksi ini |
| `balance_after` | Snapshot saldo **setelah** transaksi ini — angka yang sama juga dipakai [BALANCE.md](BALANCE.md) buat baris terakhir |

List kosong `[]` kalau gak ada transaksi di rentang tanggal itu (bukan error).

## Catatan

- Sama persis query/aturan `GetBalanceHistory()` di `sudocore2` (`backend/modules/master/member/member_services.go`) — cuma di-scope ke member yang lagi login lewat session, bukan terima `member_id` dari param kayak endpoint ERP-nya.
- Baris yang `is_deleted = true` gak ikut muncul.
- Urutan **terbaru duluan** (`created_at DESC, id DESC`).

## Tervalidasi

**⚠️ Belum tervalidasi lewat HTTP request** (2026-08-21) — query tervalidasi sintaksnya lewat `psql` (jalan tanpa error), tapi `member_balance_ledger` **kosong total** di seluruh sistem saat ini, jadi belum ada baris asli buat mastiin bentuk output. `go build`/`go vet` bersih. Update bagian ini kalau udah ada data & dites.
