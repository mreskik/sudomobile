package topup

import (
	"strings"
	"time"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type Handler interface {
	Create(c fiber.Ctx) error
	CheckStatus(c fiber.Ctx) error
	History(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

// Create: POST /account/balance/topup -- PROTECTED (wajib Authorization), member_id dari session
// token. Top-up itu wajib login (BEDA dari order yang sekarang publik, lihat CREATE ORDER.md) --
// gak ada skenario "top-up tanpa identitas", saldo yang nambah harus jelas kepunyaan siapa.
func (h *handler) Create(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	var req createTopupRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body tidak valid"))
	}

	data, errMsg, err := CreateTopup(c.Context(), h.db, memberID, req)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal membuat top up"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	return c.JSON(res.Success().SetData(data))
}

// CheckStatus: GET /account/balance/topup/:reference_number/status -- PROTECTED, dipoll sambil
// QR ditampilin ke customer. Dicek juga topup itu milik memberID yang lagi login (BEDA dari
// payment-status order yang sekarang publik lewat order_number -- top-up SELALU protected, jadi
// tetap ada guard kepemilikan di sini, konsisten sama alasan "wajib login" di atas).
func (h *handler) CheckStatus(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)
	referenceNumber := c.Params("reference_number")

	data, errMsg, err := CheckTopupStatus(c.Context(), h.db, memberID, referenceNumber)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal cek status top up"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	return c.JSON(res.Success().SetData(data))
}

// History: GET /account/balance/topup/history -- PROTECTED. SEMUA percobaan top-up member yang
// lagi login (pending/paid/expired/cancel/failed), BEDA dari BALANCE HISTORY.md
// (account/balance_handler.go) yang cuma nampilin transaksi yang UDAH settlement. Aturan
// start_date/end_date SAMA PERSIS BalanceHistory()/PointHistory() (kosong dua-duanya = hari ini).
func (h *handler) History(c fiber.Ctx) error {
	res := helpers.NewResponse()
	memberID := middleware.MemberID(c)

	today := time.Now().Format("2006-01-02")
	startDate := strings.TrimSpace(c.Query("start_date"))
	if startDate == "" {
		startDate = today
	}
	endDate := strings.TrimSpace(c.Query("end_date"))
	if endDate == "" {
		endDate = today
	}

	list, err := GetTopupHistory(c.Context(), h.db, memberID, startDate, endDate)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil riwayat top up"))
	}

	return c.JSON(res.Success().SetData(list))
}
