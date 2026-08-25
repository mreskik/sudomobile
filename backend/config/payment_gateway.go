package config

import "os"

// PaymentGatewayEndpoint: base URL service `payment` (dev/payment/, Go standalone, port default
// 98, provider Midtrans) -- sudomobile manggil service ini LANGSUNG (bukan integrasi Midtrans
// sendiri dari nol), sama pola kayak POS Laravel (PAYMENT_GATEWAY_ENDPOINT di
// App\Services\PaymentGatewayServices). Env var sama namanya biar konsisten lintas repo.
var PaymentGatewayEndpoint = "http://localhost:98"

func InitPaymentGatewayEndpoint() {
	if v := os.Getenv("PAYMENT_GATEWAY_ENDPOINT"); v != "" {
		PaymentGatewayEndpoint = v
	}
}
