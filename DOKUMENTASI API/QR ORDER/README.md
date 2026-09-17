# QR Order API

Folder ini buat API fasilitas **QR Order** di `sudomobile` — customer scan QR (mis. di
meja/outlet), langsung masuk alur pesan tanpa install/login app member. Jalur terpisah dari API
member di [`../MOBILE/`](../MOBILE/README.md), tapi 1 backend & 1 DB yang sama.

**Status (2026-09-17): RANCANGAN** — belum ada endpoint, migration, maupun kode. Yang ada baru
ketentuan/aturan mainnya.

## Daftar dokumen

- [`KETENTUAN QR ORDER.md`](./KETENTUAN%20QR%20ORDER.md) — ketentuan/mekanisme menyeluruh
  (identitas request `db_code`/`company_code`/`branch_code`, auth, meja, menu, order &
  pembayaran) + daftar hal yang belum diputusin. **Baca ini dulu.**
- Spek per endpoint — nyusul, 1 file per endpoint (pola sama kayak `../MOBILE/`).

## Riwayat

- 2026-09-17 — folder dibikin, ketentuan awal ditulis (belum ada kode).
