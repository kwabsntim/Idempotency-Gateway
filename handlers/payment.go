package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/yourusername/idempotency-gateway/database"
	"github.com/yourusername/idempotency-gateway/models"
)

type contextKey string

const RecordKey contextKey = "record"

type PaymentHandler struct {
	Store           *database.Store
	ProcessingDelay time.Duration
}

func (h *PaymentHandler) ProcessPayment(w http.ResponseWriter, r *http.Request) {
	var req models.PaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	//this is a delay to simulate the round trip to a payment gateway
	time.Sleep(h.ProcessingDelay)

	resp := models.PaymentResponse{
		Message: fmt.Sprintf("Charged %.0f %s", req.Amount, req.Currency),
		Status:  "success",
	}

	record := r.Context().Value(RecordKey).(*models.IdempotencyRecord)
	record.Response = resp
	record.StatusCode = http.StatusCreated
	record.IsProcessing = false
	close(record.Done) // unblocks ALL in-flight waiting goroutines at once

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func SetRecord(r *http.Request, record *models.IdempotencyRecord) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), RecordKey, record))
}
