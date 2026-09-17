package order

import (
	"context"
	"strings"

	"sudomobile/backend/heartbeat"
	"sudomobile/backend/helpers"
	"sudomobile/backend/modules/branch"
	"sudomobile/backend/modules/qrorder"
	"sudomobile/backend/pricing"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

// qrCreateOrderRequest: MIRIP createOrderRequest (member app) tapi TANPA branch_id/
// visit_purpose_id (dari 4 kode di query, lewat qrorder.Resolve()) dan TANPA member_id (gak ada
// login) -- order_name WAJIB (identitas tamu, gantiin member_id). use_promo_ids TETAP ada di
// struct biar bisa DITOLAK eksplisit kalau diisi (bukan diem-diem diabaikan), belum didukung di
// QR Order sama sekali (keputusan sesi 2026-09-17).
//
// order_name (BUKAN customer_name, migration sudocore2 211, 2026-09-17) -- disamain sama nama
// kolom yang UDAH ADA di POS, tr_order.order_name, biar pas jalur pull mb_order -> tr_order
// digarap gak perlu mapping/alias nama field sama sekali.
type qrCreateOrderRequest struct {
	OrderName           string        `json:"order_name"`
	CustomerPhoneNumber string        `json:"customer_phone_number"`
	PaymentMethodID     int64         `json:"payment_method_id"`
	Items               []itemRequest `json:"items"`
	UsePromoIDs         []int64       `json:"use_promo_ids"`
}

// qrCreateOrderResult: createOrderResult (member app) DITAMBAH order_source/order_name -- gak
// ada blok company/branch/visit_purpose resolusi (beda dari rencana GET VISIT PURPOSE DETAIL
// versi QR Order) karena client CREATE udah tau apa yang dia kirim, gak perlu di-echo balik.
type qrCreateOrderResult struct {
	OrderNumber   string           `json:"order_number"`
	Status        string           `json:"status"`
	SubTotal      string           `json:"sub_total"`
	TotalTax      string           `json:"total_tax"`
	TotalDiscount string           `json:"total_discount"`
	TotalBilling  string           `json:"total_billing"`
	Items         []calculatedItem `json:"items"`
	Payment       paymentInfo      `json:"payment"`
	OrderSource   string           `json:"order_source"`
	OrderName     string           `json:"order_name"`
}

// QRHandler: interface TERPISAH dari Handler (member app) -- QR Order PUBLIK total, gak ada
// middleware.Auth/AppSetting sama sekali, route-nya didaftarin di group beda (router.go).
type QRHandler interface {
	Create(c fiber.Ctx) error
	PaymentStatus(c fiber.Ctx) error
	Calculate(c fiber.Ctx) error
	GetDetail(c fiber.Ctx) error
}

type qrHandler struct {
	db *bun.DB
}

func NewQRHandler(db *bun.DB) QRHandler {
	return &qrHandler{db: db}
}

// Create: POST /api/qr-order/create-order?db_code=&company_code=&branch_code=&visit_purpose_code=
// -- versi QR Order dari Create() (member app, order_create_handler.go) di atas. REUSE LANGSUNG
// calculateOrder()/generateOrderNumber()/requestPaymentForOrder()/insertOrderItems() -- fungsi
// yang SAMA PERSIS, cuma header mb_order & identitas request yang beda (tamu vs member, 4 kode
// vs token+X-App-Setting). Lihat DOKUMENTASI API/QR ORDER/CREATE ORDER.md & KETENTUAN QR
// ORDER.md buat urutan validasi & alasan tiap keputusan.
func (h *qrHandler) Create(c fiber.Ctx) error {
	res := helpers.NewResponse()
	ctx := c.Context()

	qrCtx, errMsg, err := qrorder.Resolve(ctx, h.db,
		c.Query("db_code"), c.Query("company_code"), c.Query("branch_code"), c.Query("visit_purpose_code"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal validasi identitas request"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	var body qrCreateOrderRequest
	if err := c.Bind().Body(&body); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("body tidak valid"))
	}

	body.OrderName = strings.TrimSpace(body.OrderName)
	if body.OrderName == "" {
		return c.JSON(res.SetCode(100).SetMessage("order_name wajib diisi"))
	}
	if len(body.Items) == 0 {
		return c.JSON(res.SetCode(100).SetMessage("items tidak boleh kosong"))
	}
	if body.PaymentMethodID == 0 {
		return c.JSON(res.SetCode(100).SetMessage("payment_method_id wajib diisi"))
	}
	// Promo belum didukung sama sekali di QR Order (keputusan sesi 2026-09-17) -- DITOLAK
	// eksplisit kalau diisi, bukan diem-diem diabaikan (client gak boleh ngira diskonnya kepake).
	if len(body.UsePromoIDs) > 0 {
		return c.JSON(res.SetCode(100).SetMessage("promo belum didukung di QR Order"))
	}

	// Barrier operasional -- SAMA PERSIS Create() member app (branch_handler.go/heartbeat.go),
	// dicek SETELAH validasi body (fail-fast dulu buat kesalahan input yang murah dicek).
	if !branch.IsOpenNow(ctx, h.db, qrCtx.BranchID) {
		return c.JSON(res.SetCode(100).SetMessage("cabang sedang tutup (di luar jam operasional)"))
	}
	if !heartbeat.IsOnline(ctx, h.db, qrCtx.BranchID) {
		return c.JSON(res.SetCode(100).SetMessage("cabang sedang offline, coba lagi nanti"))
	}

	// calculateOrder() BUTUH memberID cuma buat cabang promo (fetchMemberPromoContext) -- UsePromoIDs
	// udah dipastikan kosong di atas, jadi cabang itu gak pernah kesentuh. memberID=0 aman.
	calcReq := calculateRequest{
		BranchID:       qrCtx.BranchID,
		VisitPurposeID: qrCtx.VisitPurposeID,
		Items:          body.Items,
	}
	calcResult, errMsg, err := calculateOrder(ctx, h.db, calcReq, 0)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal menghitung order"))
	}
	if errMsg != "" {
		return c.JSON(res.SetCode(100).SetMessage(errMsg))
	}

	paymentMethod, err := pricing.ResolvePaymentMethod(ctx, h.db, body.PaymentMethodID, qrCtx.BranchID, qrCtx.VisitPurposeID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data payment method"))
	}
	if paymentMethod == nil {
		return c.JSON(res.SetCode(100).SetMessage("payment method tidak ditemukan / tidak berlaku"))
	}

	orderType := resolveQROrderType(ctx, h.db, qrCtx.VisitPurposeID)
	orderNumber := generateOrderNumber(qrCtx.BranchCode)
	companyID := qrCtx.CompanyID
	if err := insertQROrder(ctx, h.db, orderNumber, &companyID, qrCtx.BranchID, qrCtx.VisitPurposeID, orderType, body.OrderName, nullIfEmpty(body.CustomerPhoneNumber), calcResult); err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal menyimpan order"))
	}

	payment := requestPaymentForOrder(ctx, h.db, orderNumber, qrCtx.BranchID, body.PaymentMethodID, paymentMethod.PaymentGatewayCode, calcResult.TotalBilling)

	return c.JSON(res.Success().SetData(qrCreateOrderResult{
		OrderNumber:   orderNumber,
		Status:        "pending",
		SubTotal:      calcResult.SubTotal,
		TotalTax:      calcResult.TotalTax,
		TotalDiscount: calcResult.TotalDiscount,
		TotalBilling:  calcResult.TotalBilling,
		Items:         calcResult.Items,
		Payment:       payment,
		OrderSource:   "qr",
		OrderName:     body.OrderName,
	}))
}

// resolveQROrderType: order_type mb_order buat QR Order DIRESOLVE dari
// master_visit_purpose.kiosk_mode (BUKAN hardcode 'takeaway' kayak member app -- QR Order di
// meja itu dine-in secara alami, keputusan sesi 2026-09-17). Fallback 'dinein' kalau kiosk_mode
// NULL/belum di-set admin, ATAU gagal query -- SENGAJA gak pernah gagal Create cuma gara-gara
// field info ini, order tetep harus bisa dibuat.
func resolveQROrderType(ctx context.Context, db *bun.DB, visitPurposeID int) string {
	var kioskMode *string
	_ = db.NewRaw(`SELECT kiosk_mode FROM master_visit_purpose WHERE id = ?`, visitPurposeID).Scan(ctx, &kioskMode)
	if kioskMode != nil && (*kioskMode == "dinein" || *kioskMode == "takeaway") {
		return *kioskMode
	}
	return "dinein"
}

// insertQROrder: versi QR Order dari insertOrder() (order_create_handler.go) -- header mb_order
// BEDA (member_id NULL, order_name wajib, order_source='qr', order_type diresolve BUKAN
// hardcode), detail item-nya REUSE insertOrderItems() yang SAMA PERSIS dipakai member app.
func insertQROrder(
	ctx context.Context, db *bun.DB, orderNumber string, companyID *int,
	branchID, visitPurposeID int, orderType, orderName string, customerPhone *string,
	result *calculateResult,
) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewRaw(`
			INSERT INTO mb_order (
				order_number, branch_id, member_id, visit_purpose_id, order_type, pax, status,
				order_fee, service_charge, platform_fee, delivery_cost,
				sub_total, total_discount, total_tax, total_billing,
				flag_inclusive_tax, customer_phone_number, company_id, order_source, order_name
			) VALUES (?, ?, NULL, ?, ?, NULL, 'pending', 0, 0, 0, 0, ?, ?, ?, ?, ?, ?, ?, 'qr', ?)
		`, orderNumber, branchID, visitPurposeID, orderType,
			result.SubTotal, result.TotalDiscount, result.TotalTax, result.TotalBilling,
			result.FlagInclusiveTax, customerPhone, companyID, orderName,
		).Exec(ctx)
		if err != nil {
			return err
		}

		return insertOrderItems(ctx, tx, orderNumber, result)
	})
}
