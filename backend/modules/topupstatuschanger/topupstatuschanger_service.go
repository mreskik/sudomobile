// Package topupstatuschanger: background job -- sinkronin member_topup_online.status buat
// top-up yang QR pembayarannya udah kadaluarsa tapi customer-nya gak pernah balik lagi manggil
// check-status (yang biasanya jadi titik sinkronisasi). SENGAJA gak dinamain "topupexpiry" --
// hasil akhirnya BUKAN cuma 'expired', race guard di CheckTopupStatus() bisa aja nemuin gateway-nya
// ternyata udah 'settlement' (customer sempet bayar tepat sebelum sweep jalan), jadi top-up
// di-FINALIZE jadi 'paid', bukan di-mark expired. Nama "expiry" nyesetin, seolah cuma 1 arah hasil.
//
// Pola-nya niru PERSIS orderexpiry (RunLoop()/RunOnce(), reuse fungsi inti yang sama dipakai
// endpoint) -- lihat DOKUMENTASI BACKGROUND JOB/POLA UMUM.md.
//
// Kenapa job ini perlu (2026-09-22): sebelum ini, member_topup_online gak punya jaring pengaman
// sama sekali -- beda dari order yang udah dijaga orderexpiry. Top-up yang gak pernah dipoll
// ulang bisa nyangkut 'pending' SELAMANYA. Gap ini juga ada di sisi POS Laravel
// (KioskCheckPendingPayment cuma nyapu tr_order, gak nyentuh member_topup_online) -- tapi
// diperbaiki di sini dulu (sudomobile) karena modul topup baru aja dibuat di sini, dan
// member_topup_online itu 1 tabel GLOBAL (dipakai bareng Kiosk/POS/mobile, gak ada boundary
// source) -- sweep dari sudomobile otomatis ikut nyapu top-up dari Kiosk juga, gak perlu job
// terpisah lagi di Laravel buat nutup gap yang sama.
package topupstatuschanger

import (
	"context"
	"log"
	"time"

	"sudomobile/backend/config"
	"sudomobile/backend/modules/topup"
)

// RunLoop: jalan di goroutine terpisah (dipanggil `go topupstatuschanger.RunLoop()` dari
// main.go, SAMA proses/binary kayak HTTP server-nya). Interval 1 MENIT -- SENGAJA lebih cepat
// dari konvensi orderexpiry/pointcheck/memberbalancejurnal (5 menit) -- top-up itu duit yang
// lagi ditunggu customer real-time (beda dari order yang biasanya udah checkout & ninggalin
// app), jadi celah "nyangkut pending" harus ketutup lebih cepat.
func RunLoop() {
	interval := 1 * time.Minute
	log.Println("topupstatuschanger: RunLoop jalan, interval", interval)

	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Println("topupstatuschanger: panic ke-recover:", r)
				}
			}()

			if err := RunOnce(context.Background()); err != nil {
				log.Println("topupstatuschanger: error:", err)
			}
		}()

		time.Sleep(interval)
	}
}

// RunOnce: 1 putaran sweep -- EXPORTED biar bisa dipanggil manual (mis. endpoint admin, atau
// testing) tanpa nunggu jadwal RunLoop(). Alurnya:
//  1. Cari kandidat: member_topup_online.status='pending' DAN expired_at-nya udah lewat waktu
//     sekarang (SEMUA source -- pos/kiosk/mobile, tabel ini global, gak dibatasi asal).
//  2. Tiap kandidat, panggil topup.CheckTopupStatus() -- FUNGSI YANG SAMA dipakai endpoint
//     check-status, BUKAN diimplementasi ulang di sini. Otomatis race-guard-aware: kalau
//     ternyata pas di-live-check status gateway-nya udah 'settlement' (customer sempet bayar
//     tepat sebelum sweep ini jalan), top-up di-finalize jadi 'paid' (saldo ke-update), BUKAN
//     ke-mark 'expired' keliru -- INI ALASAN kenapa job ini gak dinamain "expiry" (lihat komentar
//     package di atas).
func RunOnce(ctx context.Context) error {
	start := time.Now()
	log.Println("topupstatuschanger: mulai jalan")

	synced := 0
	defer func() {
		log.Printf("topupstatuschanger: selesai (durasi %s, %d topup disinkronin)\n", time.Since(start), synced)
	}()

	candidates, err := fetchCandidateTopups(ctx)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		log.Println("topupstatuschanger: gak ada topup kandidat, skip")
		return nil
	}
	log.Println("topupstatuschanger:", len(candidates), "topup kandidat ke-temu")

	for _, c := range candidates {
		// memberID diisi dari MemberID baris kandidat sendiri (bukan dari session/token, job ini
		// jalan server-side tanpa request customer) -- CheckTopupStatus() cek kepemilikan
		// (row.MemberID == memberID), otomatis lolos karena memang milik member itu.
		result, errMsg, err := topup.CheckTopupStatus(ctx, config.DB, c.MemberID, c.ReferenceNumber)
		if err != nil {
			log.Println("topupstatuschanger: gagal sync topup", c.ReferenceNumber, ":", err)
			continue
		}
		if errMsg != "" {
			log.Println("topupstatuschanger: skip topup", c.ReferenceNumber, ":", errMsg)
			continue
		}
		log.Println("topupstatuschanger: topup", c.ReferenceNumber, "disinkronin jadi", result.Status)
		synced++
	}

	return nil
}

type candidateTopup struct {
	ReferenceNumber string `bun:"reference_number"`
	MemberID        int64  `bun:"member_id"`
}

// fetchCandidateTopups: member_topup_online SENGAJA gak difilter `source` (pos/kiosk/mobile) --
// tabel ini GLOBAL, 1 percobaan = 1 baris (beda dari mb_order_payment_request yang bisa lebih
// dari 1 attempt per order), jadi gak butuh JOIN LATERAL kayak fetchCandidateOrders().
func fetchCandidateTopups(ctx context.Context) ([]candidateTopup, error) {
	rows := []candidateTopup{}
	err := config.DB.NewRaw(`
		SELECT reference_number, member_id
		FROM member_topup_online
		WHERE status = 'pending'
			AND expired_at IS NOT NULL
			AND expired_at < now()
	`).Scan(ctx, &rows)
	return rows, err
}
