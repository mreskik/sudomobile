package order

import (
	"strconv"
	"time"

	"github.com/google/uuid"
)

// generateOrderNumber: format "MB" (mobile) + branch_id + timestamp (YmdHis) + 6 hex acak.
// Beda dari OrderServices::GenerateOrderNumber() POS ("NO" + terminal_id + branch_code +
// timestamp) -- sudomobile gak punya terminal_id (gak ada mesin kasir), dan branch_code gak
// dipakai (branch_id numerik udah cukup unik). 6 hex acak ditambahin sebagai pengaman
// collision -- beda dari POS yang aman ngandelin terminal_id (1 device = gak mungkin nembak 2
// request bersamaan di detik yang sama), request mobile bisa concurrent dari banyak device
// sekaligus buat branch yang sama, timestamp doang gak cukup unik.
func generateOrderNumber(branchID int) string {
	return "MB" + strconv.Itoa(branchID) + time.Now().Format("20060102150405") + uuid.NewString()[:6]
}

// generateULID: dipakai buat mb_order_detail.ulid/mb_order_detail_package.ulid. POS pakai
// Str::ulid() (Laravel, format ULID asli yang lexicographically sortable by time) -- di sini
// pakai UUID v4 biasa (github.com/google/uuid, udah ada di go.mod, gak nambah dependency baru)
// karena sortability-nya emang gak dipakai/gak dibutuhin logic manapun, cuma butuh unik.
func generateULID() string {
	return uuid.NewString()
}
