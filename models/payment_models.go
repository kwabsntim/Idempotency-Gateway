package models

import "time"

type PaymentRequest struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type PaymentResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

type IdempotencyRecord struct {
	RequestHash  string
	Response     PaymentResponse
	StatusCode   int
	IsProcessing bool
	CreatedAt    time.Time
	Done         chan struct{}
}
