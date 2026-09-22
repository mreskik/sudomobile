# Account - Top Up History

Riwayat **SEMUA percobaan top-up** member yang lagi login, terbaru duluan. **PROTECTED**. **BEDA** dari [`BALANCE HISTORY.md`](../BALANCE%20HISTORY.md) yang cuma nampilin transaksi yang **UDAH settlement** (baca `member_balance_ledger`, baris situ baru ada abis `paid`). Endpoint ini baca `member_topup_online` **langsung** — percobaan `pending`/`expired`/`cancel`/`failed` juga ikut muncul, jadi customer bisa liat "topup gue kemarin kenapa gak masuk-masuk".

```
GET /api/account/balance/topup/history?start_date=2026-09-01&end_date=2026-09-22
Authorization: Bearer <token>
```

## Parameter Query (opsional)

Sama persis aturan [`BALANCE HISTORY.md`](../BALANCE%20HISTORY.md)/[`POINT HISTORY.md`](../POINT%20HISTORY.md) — `start_date`/`end_date` (format `YYYY-MM-DD`), **dua-duanya kosong → default HARI INI**, filter berdasarkan `created_at`, inklusif `end_date`.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "reference_number": "TUSBE2026092210004571",
      "amount": "100000.00",
      "status": "paid",
      "created_at": "2026-09-22T10:10:35+07:00",
      "paid_at": "2026-09-22T10:15:12+07:00"
    },
    {
      "reference_number": "TUSBE2026092108001234",
      "amount": "50000.00",
      "status": "expired",
      "created_at": "2026-09-21T14:00:00+07:00"
    }
  ]
}
```

`paid_at` cuma keisi kalau `status == "paid"`. List kosong `[]` kalau gak ada percobaan top-up di rentang tanggal itu (bukan error).

## Sumber data / implementasi

- `sudomobile/backend/modules/topup/topup_service.go` — `GetTopupHistory()`.
- `sudomobile/backend/modules/topup/topup_handler.go` — `History()`.
- Route `/balance/topup/history` didaftarkan **sebelum** `/balance/topup/:reference_number/status` di `backend/router.go` — kalau kebalik, path ini bakal ketangkep sama route dinamis itu duluan (`reference_number="history"`).

## Status

**Baru dibuat (2026-09-22), belum tervalidasi live** — `go build`/`go vet` bersih, belum dites end-to-end lewat HTTP request beneran. Update bagian ini setelah dites.
