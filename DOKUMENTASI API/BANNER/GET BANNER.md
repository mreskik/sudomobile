# Banner - Get Banner

```
GET /api/banner
```

**Publik** (gak butuh `Authorization`) — semua section banner mobile customer app digabung 1 endpoint: splash, quick action (kiri-kanan), login sheet, + 4 daftar banner (swipe/popup/promotion/about us). Dipanggil sekitar awal-buka app / home, 1 round-trip.

Wajib header `X-App-Setting` (sama kayak semua route lain), **scoping-nya pakai `brand_id`** dari header itu — bukan branch, bukan company.

## Request

Gak ada body/param. Cukup header `X-App-Setting` yang isinya `brand_id`.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "banner_splash_src": "/storage/uploads/images/splash-new.jpg",
    "banner_quick_action_left_button_src": null,
    "banner_quick_action_right_button_src": null,
    "banner_login_sheet_src": "/storage/uploads/images/login-old.jpg",
    "banner_swipe": [
      { "banner_src": "/storage/uploads/images/swipeB1.jpg", "name": "Swipe B1" },
      { "banner_src": "/storage/uploads/images/swipeA2.jpg", "name": "Swipe A2" },
      { "banner_src": "/storage/uploads/images/swipeA1.jpg", "name": "Swipe A1" }
    ],
    "banner_popup": [
      { "banner_src": "/storage/uploads/images/popup1.jpg", "action_link": "https://example.com/promo" }
    ],
    "banner_promotion": [
      { "banner_src": "/storage/uploads/images/promo1.jpg", "name": "Promo 1" }
    ],
    "banner_about_us": [
      { "banner_src": "/storage/uploads/images/about1.jpg", "name": "Tentang Kami 1" }
    ]
  }
}
```

`banner_splash_src` & `banner_login_sheet_src` di contoh atas SENGAJA asalnya dari 2 campaign yang beda (lihat "Logic per bagian" di bawah) — bukan typo, itu contoh nyata perilaku "tiap kolom nyari sendiri-sendiri".

Semua field bisa kosong (`null` buat 4 slot gambar tunggal, `[]` buat 4 daftar) kalau belum ada campaign (`master_image_mb_cust`) yang aktif & cocok scope brand-nya — bukan error, cuma berarti belum ada banner buat brand ini.

## Sumber data

Baca langsung dari DB `sudocore2` (`sudomobile` connect ke DB yang sama, gak ada sync/bridge layer kayak POS↔APIANDORDER) — tabel `master_image_mb_cust` + `master_image_mb_cust_brands` + 4 tabel anak (`_banner_swipe`/`_banner_popup`/`_banner_promotion`/`_banner_about_us`). Lihat `MASTER IMAGE MOBILE CUSTOMER.md` di sudocore2 buat skema aslinya.

**File gambarnya** dibalikin path lokal (`/storage/uploads/images/...`) — `sudomobile` mount langsung ke folder storage `sudocore2` (env `STORAGE_PATH`, lihat `CATATAN INTERNAL.md`), gak ada proses download/copy kayak POS. Frontend prefix sendiri pakai base URL `sudomobile`.

## Logic per bagian

**Scope** — 1 campaign (`master_image_mb_cust`) dianggap "cocok" kalau `is_active = true` DAN (`flag_all_brand = true` ATAU ada baris di `master_image_mb_cust_brands` yang `brand_id`-nya cocok sama `brand_id` di `X-App-Setting`).

- **4 slot gambar tunggal** (`banner_splash_src`, `banner_quick_action_left_button_src`, `banner_quick_action_right_button_src`, `banner_login_sheet_src`) — **TIAP KOLOM NYARI SENDIRI-SENDIRI** (2026-08-24, revisi dari desain awal yang "1 campaign buat 4 kolom sekaligus"). Buat tiap kolom: dipertimbangin cuma campaign yang **lagi aktif HARI INI** (kena filter tanggal yang sama kayak poin di bawah -- `flag_all_date = true` ATAU tanggal sekarang di antara `date_start`-`date_end`), diambil dari yang **paling baru** yang kolom itu **gak `null`**. Kalau campaign teraktif-terbaru kolomnya kosong buat kolom ini, turun ke campaign aktif berikutnya yang lebih lama, dst -- bukan langsung `null`. Efeknya: **4 kolom ini bisa aja asalnya dari 4 campaign yang beda-beda**, gak harus 1 sumber yang sama. Campaign yang gak aktif hari ini (`flag_all_date=false` & di luar rentang) gak pernah dipertimbangin buat kolom manapun, walau dia paling baru dibuat.
- **4 daftar banner** (`banner_swipe`/`banner_popup`/`banner_promotion`/`banner_about_us`) — SEBALIKNYA, diambil dari **SEMUA campaign yang cocok** (gak dibatasin ke 1 campaign kayak header), diurutkan **campaign paling baru duluan** (`mimc.id DESC`), baru di dalam 1 campaign yang sama urut `sequence` ASC. Efeknya: banner dari campaign terbaru selalu nongol duluan, banner dari campaign lama (yang masih aktif) tetep ikut muncul di bawahnya, bukan ketutup/hilang. `sequence` **gak diikutin di response** (2026-08-24) — datanya udah kekirim urut dari server, frontend cukup render apa adanya sesuai urutan array, gak perlu sort ulang atau tau angka mentahnya.
- **Filter tanggal aktif PER CAMPAIGN** (2026-08-24, cuma buat `banner_swipe`/`banner_popup`/`banner_promotion` — **BUKAN** `banner_about_us`) — `flag_all_date`/`date_start`/`date_end` ada di **HEADER `master_image_mb_cust`** (bukan di tabel banner-nya, 1 jadwal berlaku buat SATU campaign, bukan per baris banner). Campaign cuma ikut disertakan buat 3 kategori itu kalau `flag_all_date = true` ATAU tanggal sekarang ada di antara `date_start`-`date_end` campaign itu -- campaign yang gak lolos, SEMUA banner swipe/popup/promotion-nya gak ikut muncul (banner dari campaign lain yang lolos tetep normal). Kolom-kolom ini sendiri **gak ikut di response** (cuma dipakai buat filter di query, sama kayak `sequence`) — kalau `flag_all_date = false` tapi `date_start`/`date_end` kosong (belum diisi admin), campaign itu dianggap **TIDAK aktif** buat 3 kategori ini (gak muncul), bukan "selalu aktif". `banner_about_us` **TIDAK** kena filter ini sama sekali -- tetep muncul walau campaign-nya lagi "gak tayang".

## Tervalidasi live (2026-08-24)

**Multi-campaign + scoping brand**: Insert 2 campaign test langsung via `psql`: Campaign A (`flag_all_brand=true`, 2 banner swipe sequence 2 & 1) dan Campaign B (`flag_all_brand=false`, scoped ke `brand_id=6`, id lebih baru, 1 banner swipe) → request dengan `brand_id=6` → header balikin punya Campaign B (paling baru), `banner_swipe` balikin urutan `[Swipe B1, Swipe A2, Swipe A1]` (Campaign B duluan, lalu Campaign A urut sequence) — sesuai desain. Request ulang dengan `brand_id=24` (gak match scope Campaign B) → cuma Campaign A yang muncul (header & swipe), Campaign B ke-filter bener. Data test dibersihin abis verifikasi.

**Filter tanggal aktif per campaign**: Insert 4 campaign test (bukan 4 baris banner dalam 1 campaign — kolomnya di header), masing-masing skenario: (1) `flag_all_date=true` → banner swipe-nya muncul, (2) `flag_all_date=false`, tanggal sekarang di dalam `date_start`-`date_end` → muncul, (3) `flag_all_date=false`, tanggal sekarang di LUAR range → gak muncul, (4) `flag_all_date=false`, `date_start`/`date_end` kosong → gak muncul (fallback aman). Hasil: `banner_swipe` cuma balikin banner dari campaign (1) dan (2), persis sesuai desain. Sekalian dibuktiin `banner_about_us` dari campaign (3) yang "expired" **tetep muncul** di response (isolasi about_us dari filter tanggal beneran jalan). Data test dibersihin abis verifikasi.

**Header per-kolom independen (2026-08-24, revisi)**: Insert 3 campaign test: Campaign 1 (lebih lama, aktif hari ini, cuma isi `banner_login_sheet_src`), Campaign 2 (lebih baru dari 1, aktif hari ini, cuma isi `banner_splash_src`), Campaign 3 (PALING baru dari semua, tapi expired -- isi KEDUA kolom itu). Hasil: `banner_splash_src` balikin punya Campaign 2 (terbaru yang aktif & ngisi kolom itu), `banner_login_sheet_src` balikin punya Campaign 1 (fallback karena Campaign 2 kosong buat kolom itu) — Campaign 3 gak kepakai sama sekali buat kolom manapun walau dia paling baru & isinya lengkap, karena gak lolos filter tanggal. Persis sesuai desain "tiap kolom nyari sendiri, skip campaign yang kosong/gak aktif". Data test dibersihin abis verifikasi.
