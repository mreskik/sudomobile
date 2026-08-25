package order

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"sudomobile/backend/config"
)

// createQrisRequest/paymentGatewayResponse: mirror PERSIS DTO service `payment`
// (payment/backend/modules/paymentgateway/paymentgateway_dto.go) -- CompanyID SENGAJA gak ada
// field-nya, service yang resolve sendiri dari BranchID (lihat PaymentGatewayService
// .CreateQrisPayment, dijelasin di sana kenapa company_id gak boleh dipercaya dari caller).
type createQrisRequest struct {
	OrderID            string `json:"order_id"`
	PaymentGatewayCode string `json:"payment_gateway_code"`
	Amount             int64  `json:"amount"`
	BranchID           *int   `json:"branch_id"`
}

type paymentGatewayResponse struct {
	OrderID            string  `json:"order_id"`
	PaymentGatewayCode string  `json:"payment_gateway_code"`
	Provider           string  `json:"provider"`
	Channel            string  `json:"channel"`
	Amount             string  `json:"amount"`
	Status             string  `json:"status"`
	VendorQRString     *string `json:"vendor_qr_string"`
	VendorQRURL        *string `json:"vendor_qr_url"`
	VendorVA           *string `json:"vendor_va"`
	ExpiredAt          *string `json:"expired_at"`
	SettlementAt       *string `json:"settlement_at"`
}

type paymentGatewayEnvelope struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    paymentGatewayResponse `json:"data"`
}

// requestQrisPayment: proxy ke POST {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/qris -- mirror
// PaymentGatewayServices::RequestPayment() POS. Timeout pendek (10 detik) sengaja dipasang --
// ini dipanggil SINKRON di tengah alur save-order, gak boleh nge-hang lama nunggu service lain.
func requestQrisPayment(orderID, paymentGatewayCode string, amount int64, branchID int) (*paymentGatewayResponse, error) {
	body, err := json.Marshal(createQrisRequest{
		OrderID:            orderID,
		PaymentGatewayCode: paymentGatewayCode,
		Amount:             amount,
		BranchID:           &branchID,
	})
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(config.PaymentGatewayEndpoint+"/payment-gateway/qris", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var envelope paymentGatewayEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("response service payment gak valid: %w", err)
	}
	if envelope.Code != 0 {
		return nil, fmt.Errorf("service payment: %s", envelope.Message)
	}

	return &envelope.Data, nil
}

// cancelPaymentGateway: proxy ke POST {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{order_id}/cancel.
// Mirror PaymentGatewayServices::CancelPendingAttempt() POS (bagian yang beneran manggil
// Midtrans-nya).
func cancelPaymentGateway(orderID string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(config.PaymentGatewayEndpoint+"/payment-gateway/"+orderID+"/cancel", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("response service payment gak valid: %w", err)
	}
	if envelope.Code != 0 {
		return fmt.Errorf("service payment: %s", envelope.Message)
	}
	return nil
}

// getPaymentGatewayStatus: proxy ke GET {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{order_id} --
// live-check status transaksi (dipoll sambil QR ditampilin ke customer). Mirror
// PaymentGatewayServices::CheckStatus() POS.
func getPaymentGatewayStatus(orderID string) (*paymentGatewayResponse, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(config.PaymentGatewayEndpoint + "/payment-gateway/" + orderID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var envelope paymentGatewayEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("response service payment gak valid: %w", err)
	}
	if envelope.Code != 0 {
		return nil, fmt.Errorf("service payment: %s", envelope.Message)
	}

	return &envelope.Data, nil
}
