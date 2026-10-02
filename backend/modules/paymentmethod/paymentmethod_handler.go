package paymentmethod

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"sudomobile/backend/helpers"
	"sudomobile/backend/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/uptrace/bun"
)

type Handler interface {
	GetList(c fiber.Ctx) error
}

type handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) Handler {
	return &handler{db: db}
}

type paymentMethodListItem struct {
	ID         int64   `json:"id" bun:"id"`
	Name       string  `json:"name" bun:"name"`
	Code       string  `json:"code" bun:"code"`
	ColorTheme *string `json:"color_theme" bun:"color_theme"`
	// PaymentMethodTypeID/PaymentMethodTypeName (2026-10-02, BARU diekspos -- sebelumnya
	// PaymentMethodTypeID ada tapi json:"-", internal doang) -- dipakai jg GetList()/
	// QRHandler.GetList() buat deteksi item WALLET_PAYMENT (id=5, master_payment_method_type)
	// & nempelin field WalletInformation di bawah.
	PaymentMethodTypeID   *int64      `json:"payment_type_id" bun:"payment_method_type_id"`
	PaymentMethodTypeName *string     `json:"payment_type_name" bun:"payment_method_type_name"`
	WalletInformation     *walletInfo `json:"wallet_information,omitempty"`
}

// walletInfo: info tambahan KHUSUS item WALLET_PAYMENT -- code:0+member_name+balance kalau ada
// token valid (member login), code:100+message kalau gak (gak ada token / token invalid /
// QR Order yang emang gak pernah punya token). Lihat enrichWalletInfo().
//
// Balance (2026-10-02, rename dari Saldo/json "saldo") -- istilah response diseragamkan ke
// Inggris ("balance"), konsisten sama field lain di sudomobile (member_balance_ledger dll).
type walletInfo struct {
	Code       int    `json:"code"`
	Message    string `json:"message,omitempty"`
	MemberName string `json:"member_name,omitempty"`
	Balance    string `json:"balance,omitempty"`
}

// WalletPaymentMethodTypeID: id tetap master_payment_method_type.name='WALLET_PAYMENT'
// (migration sudocore2 239, 2026-10-01). EXPORTED -- dipakai bareng package order
// (order_create_handler.go) buat deteksi barrier wallet pas Create(), satu sumber kebenaran
// biar id ini gak keduplikasi di 2 tempat.
const WalletPaymentMethodTypeID int64 = 5

// GetList: daftar payment method yang bisa dipakai di mobile customer app buat 1
// branch+visit_purpose (2026-08-24). Nested di bawah endpoint visit-purpose-detail
// (`branch/:branch_id/visit-purpose/:visit_purpose_id/payment-method`) karena scoping-nya
// sama persis: branch + visit purpose.
//
// 2 preseden yang beda filosofi di POS:
//   - KioskController::GetPaymentMethodList() -- cuma filter payment_gateway_code gak kosong
//     (Kiosk self-service, gak ada kasir buat mungutin cash), GAK filter branch/visit_purpose
//     sama sekali.
//   - MasterController::GetPaymentMethod() -- filter visit_purpose lewat JOIN doang ke
//     mr_payment_method_visit_purposes, TAPI ini keliatan gak lengkap: gak nangani
//     flag_all_visitpurpose=true (payment method yang berlaku ke SEMUA visit purpose tanpa
//     baris junction) -- kemungkinan gap yang emang ada di POS, bukan sesuatu yang ditiru di
//     sini.
//
// Desain sudomobile (disepakati eksplisit 2026-08-24): filter gateway-only (samain Kiosk --
// mobile customer app itu online-order, gak ada kasir yang mungutin cash), DITAMBAH scoping
// branch+visit_purpose yang bener (flag_all_branch/flag_all_visitpurpose dihormati, mirip pola
// flag_all_brand di master_image_mb_cust) -- karena sudomobile multi-branch beda dari POS yang
// selalu 1 branch per install.
func (h *handler) GetList(c fiber.Ctx) error {
	res := helpers.NewResponse()

	branchID, err := strconv.Atoi(c.Params("branch_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("branch_id tidak valid"))
	}
	visitPurposeID, err := strconv.Atoi(c.Params("visit_purpose_id"))
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("visit_purpose_id tidak valid"))
	}

	list, err := resolvePaymentMethodList(c.Context(), h.db, branchID, visitPurposeID)
	if err != nil {
		return c.JSON(res.SetCode(100).SetMessage("gagal ambil data payment method"))
	}

	// memberID 0 kalau gak ada token / token invalid/expired (middleware.OptionalAuth, dipasang
	// di route ini -- router.go) -- enrichWalletInfo() treat 0 sebagai "butuh login".
	enrichWalletInfo(c.Context(), h.db, list, middleware.MemberID(c))

	return c.JSON(res.Success().SetData(list))
}

// resolvePaymentMethodList: query INTI GetList() di atas, DIPISAH (2026-09-17) biar dipakai
// BARENG versi QR Order (paymentmethod_qr_handler.go) tanpa duplikasi query -- SATU tempat
// nulis filter gateway-only + scoping branch/visit_purpose, siapa pun pemanggilnya.
//
// payment_method_type_id (2026-10-01) ikut di-SELECT -- dipakai enrichWalletInfo() di kedua
// pemanggil (member app & QR Order) buat deteksi item WALLET_PAYMENT.
//
// Filter gateway-only DILONGGARKAN (2026-10-01): WALLET_PAYMENT (type_id=5) SELALU lolos
// filter ini apa pun isi payment_gateway_code-nya -- payment method saldo settle langsung,
// gak lewat gateway eksternal kayak payment method lain, jadi gak wajib punya gateway code.
// Payment method lain TETAP wajib gateway_code kayak sebelumnya (gak berubah).
func resolvePaymentMethodList(ctx context.Context, db *bun.DB, branchID, visitPurposeID int) ([]paymentMethodListItem, error) {
	list := []paymentMethodListItem{}
	err := db.NewRaw(`
		SELECT DISTINCT mpm.id, mpm.name, mpm.code, mpm.color_theme,
			mpm.payment_method_type_id, mpmt.name AS payment_method_type_name
		FROM master_payment_method mpm
		LEFT JOIN master_payment_method_type mpmt ON mpmt.id = mpm.payment_method_type_id
		WHERE mpm.is_active = true AND COALESCE(mpm.is_deleted, false) = false
			AND (
				mpm.payment_method_type_id = ?
				OR (mpm.payment_gateway_code IS NOT NULL AND mpm.payment_gateway_code != '')
			)
			AND (
				mpm.flag_all_branch = true
				OR EXISTS (
					SELECT 1 FROM master_payment_method_branches b
					WHERE b.payment_method_id = mpm.id AND b.branch_id = ?
						AND COALESCE(b.is_deleted, false) = false AND b.is_active = true
				)
			)
			AND (
				mpm.flag_all_visitpurpose = true
				OR EXISTS (
					SELECT 1 FROM master_payment_method_visit_purposes vp
					WHERE vp.payment_method_id = mpm.id AND vp.visitpurpose_id = ?
						AND COALESCE(vp.is_deleted, false) = false
				)
			)
		ORDER BY mpm.name ASC
	`, WalletPaymentMethodTypeID, branchID, visitPurposeID).Scan(ctx, &list)
	return list, err
}

// enrichWalletInfo: tempelin WalletInformation KHUSUS item WALLET_PAYMENT (type_id=5), item
// lain gak disentuh (WalletInformation tetap nil, omitempty bikin field itu gak nongol di JSON).
//
// memberID 0 -> SELALU need login (code:100) -- ini mencakup 3 kondisi: token gak dikirim
// sama sekali, token dikirim tapi invalid/expired, DAN QR Order (route-nya gak pernah pasang
// middleware.Auth/OptionalAuth sama sekali, jadi middleware.MemberID(c) di situ SELALU 0 --
// WALLET_PAYMENT otomatis SELALU need login di QR Order, gak perlu kode khusus).
//
// memberID != 0 -> resolve member_name (master_member) + saldo (member_balance_ledger,
// balance_after baris terakhir, pola SAMA PERSIS account.Balance() handler) -- member gak
// pernah ada histori saldo BUKAN error, fallback "0.00".
func enrichWalletInfo(ctx context.Context, db *bun.DB, list []paymentMethodListItem, memberID int64) {
	for i := range list {
		if list[i].PaymentMethodTypeID == nil || *list[i].PaymentMethodTypeID != WalletPaymentMethodTypeID {
			continue
		}

		if memberID == 0 {
			list[i].WalletInformation = &walletInfo{Code: 100, Message: "need login"}
			continue
		}

		var memberName string
		if err := db.NewRaw(`SELECT name FROM master_member WHERE id = ?`, memberID).Scan(ctx, &memberName); err != nil {
			list[i].WalletInformation = &walletInfo{Code: 100, Message: "need login"}
			continue
		}

		var balance string
		err := db.NewRaw(`
			SELECT balance_after FROM member_balance_ledger
			WHERE member_id = ? AND is_deleted = false
			ORDER BY created_at DESC, id DESC LIMIT 1
		`, memberID).Scan(ctx, &balance)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				list[i].WalletInformation = &walletInfo{Code: 100, Message: "need login"}
				continue
			}
			balance = "0.00"
		}

		list[i].WalletInformation = &walletInfo{Code: 0, MemberName: memberName, Balance: balance}
	}
}
