# sudomobile

Backend buat aplikasi mobile customer (member point/saldo, order, promo/discount, dll).

## Modul & Endpoint

Detail lengkap tiap endpoint (request/response, error case, catatan implementasi) ada di **`DOKUMENTASI API/<NAMA MODUL>/`**.

| Modul                   | Endpoint                                                            | Dokumentasi                                             |
| ----------------------- | ------------------------------------------------------------------- | ------------------------------------------------------- |
| **Auth** (publik)       | `check_number`, `request_otp`, `register`, `login_otp`, `login_pin` | [`DOKUMENTASI API/AUTH/`](DOKUMENTASI%20API/AUTH)       |
| **Auth** (protected)    | `pin/create`, `pin/change`, `pin/reset`                             | [`DOKUMENTASI API/AUTH/`](DOKUMENTASI%20API/AUTH)       |
| **Account** (protected) | `account/me`, `account/balance(/history)`, `account/point(/history)` | [`DOKUMENTASI API/ACCOUNT/`](DOKUMENTASI%20API/ACCOUNT) |

## Menjalankan

```
go run main.go
```

Default port `101` (`APP_PORT` di `.env`).
