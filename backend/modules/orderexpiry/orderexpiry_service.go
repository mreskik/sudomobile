// Package orderexpiry: background job -- sinkronin mb_order.status jadi 'expired' buat order
// yang QR pembayarannya udah kadaluarsa tapi customer-nya gak pernah balik lagi manggil
// payment-status/order-detail (yang biasanya jadi titik sinkronisasi). Pola-nya niru PERSIS
// pointcheck di sudocore2 (RunLoop()/RunOnce(), gak pakai library cron eksternal) -- lihat
// DOKUMENTASI BACKGROUND JOB/POLA UMUM.md.
package orderexpiry

import (
	"context"
	"log"
	"time"

	"sudomobile/backend/config"
	"sudomobile/backend/modules/order"
)

// RunLoop: jalan di goroutine terpisah (dipanggil `go orderexpiry.RunLoop()` dari main.go, SAMA
// proses/binary kayak HTTP server-nya, bukan cmd/binary sendiri). Interval 5 menit -- sama kayak
// pointcheck/memberbalancejurnal (job ini sifatnya jaring pengaman, bukan yang utama nge-sync
// status real-time -- itu udah kejadian tiap polling payment-status/buka order-detail -- jadi
// gak butuh interval lebih cepat dari itu).
func RunLoop() {
	interval := 5 * time.Minute
	log.Println("orderexpiry: RunLoop jalan, interval", interval)

	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Println("orderexpiry: panic ke-recover:", r)
				}
			}()

			if err := RunOnce(context.Background()); err != nil {
				log.Println("orderexpiry: error:", err)
			}
		}()

		time.Sleep(interval)
	}
}

// RunOnce: 1 putaran sweep -- EXPORTED biar bisa dipanggil manual (mis. endpoint admin, atau
// testing) tanpa nunggu jadwal RunLoop(). Alurnya:
//  1. Cari kandidat: mb_order.status='pending' YANG attempt TERAKHIRnya (mb_order_payment_request)
//     masih status='pending' DAN expired_at-nya udah lewat waktu sekarang.
//  2. Tiap kandidat, panggil order.SyncPaymentStatus() -- FUNGSI YANG SAMA dipakai endpoint
//     payment-status/order-detail, BUKAN diimplementasi ulang di sini. Otomatis race-guard-aware:
//     kalau ternyata pas di-live-check status gateway-nya udah 'settlement' (customer sempet
//     bayar tepat sebelum sweep ini jalan), order di-finalize jadi 'paid', BUKAN ke-mark
//     'expired' keliru.
func RunOnce(ctx context.Context) error {
	start := time.Now()
	log.Println("orderexpiry: mulai jalan")

	synced := 0
	defer func() {
		log.Printf("orderexpiry: selesai (durasi %s, %d order disinkronin)\n", time.Since(start), synced)
	}()

	candidates, err := fetchCandidateOrders(ctx)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		log.Println("orderexpiry: gak ada order kandidat, skip")
		return nil
	}
	log.Println("orderexpiry:", len(candidates), "order kandidat ke-temu")

	for _, c := range candidates {
		status, _, errMsg, err := order.SyncPaymentStatus(ctx, config.DB, c.OrderNumber, c.Status)
		if err != nil {
			log.Println("orderexpiry: gagal sync order", c.OrderNumber, ":", err)
			continue
		}
		if errMsg != "" {
			log.Println("orderexpiry: skip order", c.OrderNumber, ":", errMsg)
			continue
		}
		log.Println("orderexpiry: order", c.OrderNumber, "disinkronin jadi", status)
		synced++
	}

	return nil
}

type candidateOrder struct {
	OrderNumber string `bun:"order_number"`
	Status      string `bun:"status"`
}

// fetchCandidateOrders: JOIN LATERAL ke attempt TERBARU tiap order (bukan sembarang attempt --
// order bisa punya lebih dari 1 baris mb_order_payment_request kalau nanti retry payment
// dibangun) -- exact pattern yang sama kayak query "attempt terbaru" di ORDER HISTORY.md/
// CANCEL ORDER.md.
func fetchCandidateOrders(ctx context.Context) ([]candidateOrder, error) {
	rows := []candidateOrder{}
	err := config.DB.NewRaw(`
		SELECT mo.order_number, mo.status
		FROM mb_order mo
		JOIN LATERAL (
			SELECT status, expired_at FROM mb_order_payment_request
			WHERE order_number = mo.order_number ORDER BY created_at DESC LIMIT 1
		) latest_pr ON true
		WHERE mo.status = 'pending'
			AND latest_pr.status = 'pending'
			AND latest_pr.expired_at IS NOT NULL
			AND latest_pr.expired_at < now()
	`).Scan(ctx, &rows)
	return rows, err
}
