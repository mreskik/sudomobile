package topup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"sudomobile/backend/config"
)

// createQrisRequest/paymentGatewayResponse/paymentGatewayEnvelope -- mirror PERSIS DTO service
// `payment`, SAMA seperti order/payment_gateway_client.go. Duplikasi sengaja (bukan diekstrak jadi
// shared package) -- konsisten sama pola project ini sejauh ini, tiap module order/topup megang
// client-nya sendiri-sendiri.
type createQrisRequest struct {
	OrderID            string `json:"order_id"`
	PaymentGatewayCode string `json:"payment_gateway_code"`
	Amount             int64  `json:"amount"`
	BranchID           *int64 `json:"branch_id"`
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

// requestQrisPayment: proxy ke POST {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/qris -- order_id
// di sini diisi reference_number top-up (bukan order_number), service `payment` gak beda-bedain,
// cuma butuh id unik buat identifikasi transaksi.
func requestQrisPayment(referenceNumber, paymentGatewayCode string, amount int64, branchID int64) (*paymentGatewayResponse, error) {
	body, err := json.Marshal(createQrisRequest{
		OrderID:            referenceNumber,
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

// getPaymentGatewayStatus: proxy ke GET {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{order_id} --
// live-check status transaksi, dipoll sambil QR ditampilin ke customer.
func getPaymentGatewayStatus(referenceNumber string) (*paymentGatewayResponse, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(config.PaymentGatewayEndpoint + "/payment-gateway/" + referenceNumber)
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

// cancelPaymentGateway: proxy ke POST {PAYMENT_GATEWAY_ENDPOINT}/payment-gateway/{order_id}/cancel
// -- dipakai fallback expired (mirror pola CheckStatus() APIANDORDER: expired_at lokal lewat tapi
// gateway masih 'pending' -- webhook mungkin gak akan pernah nyampe, cancel eksplisit ke gateway).
func cancelPaymentGateway(referenceNumber string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(config.PaymentGatewayEndpoint+"/payment-gateway/"+referenceNumber+"/cancel", "application/json", nil)
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
