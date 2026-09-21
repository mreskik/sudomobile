// Package heartbeat: baca branch_heartbeat (diisi APIANDORDER, dari POST /pos/heartbeat/:branch_id
// yang dikirim command `heartbeat:send` POS tiap N detik (INTERVAL_SECONDS, sekarang 5 detik --
// lihat SendHeartbeat.php) -- KALAU dayshift branch itu lagi kebuka, lihat SEND HEARTBEAT.md di
// posv1-laravel. Dipakai buat 2 hal di sudomobile
// (disepakati 2026-08-27): barrier keras nolak order kalau branch offline (order/order_create_handler.go),
// dan ikut nentuin flag_status_store_open di branch list (modules/branch/branch_handler.go).
package heartbeat

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/uptrace/bun"
)

// OfflineThreshold: idealnya 3x interval kirim command `heartbeat:send` (posv1-laravel) --
// toleransi kalau 1-2 ping kelewat karena network blip sesaat, gak langsung dianggap offline
// padahal cuma telat dikit. Disepakati 2026-08-27 (waktu itu interval kirim 30 detik, threshold
// 90 detik) -- SEKARANG dikonfigurasi lewat env HEARTBEAT_OFFLINE_THRESHOLD_SECONDS (2026-09-21),
// biar gak perlu ubah+rebuild kode tiap interval kirim POS berubah. Default tetap 90 detik kalau
// env-nya gak di-set (backward compatible), TAPI WAJIB disesuaikan manual di .env kalau interval
// kirim POS berubah -- gak ada mekanisme sinkron otomatis antara dua config ini.
var OfflineThreshold = 90 * time.Second

func InitOfflineThreshold() {
	raw := os.Getenv("HEARTBEAT_OFFLINE_THRESHOLD_SECONDS")
	if raw == "" {
		return
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return
	}
	OfflineThreshold = time.Duration(seconds) * time.Second
}

// IsOnline: true kalau branch_heartbeat.last_ping_at masih dalam OfflineThreshold dari sekarang.
// false juga (BUKAN error) kalau branch itu belum PERNAH kirim heartbeat sama sekali (belum ada
// baris di branch_heartbeat) -- "belum pernah online" dianggap sama kayak "offline" buat
// kebutuhan barrier ini, dua-duanya sama-sama berarti "gak aman nerima order sekarang".
func IsOnline(ctx context.Context, db *bun.DB, branchID int) bool {
	var lastPingAt time.Time
	// Error apa pun di sini (baris gak ada ATAU error DB beneran) -- fail-safe ke false,
	// mending nolak order daripada nerima order buat branch yang statusnya gak kebaca jelas.
	err := db.NewRaw(`SELECT last_ping_at FROM branch_heartbeat WHERE branch_id = ?`, branchID).Scan(ctx, &lastPingAt)
	if err != nil {
		return false
	}
	return time.Since(lastPingAt) < OfflineThreshold
}
