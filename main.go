package main

import (
	"log"
	"net/http"

	"github.com/yourusername/idempotency-gateway/config"
	"github.com/yourusername/idempotency-gateway/database"
	"github.com/yourusername/idempotency-gateway/handlers"
	"github.com/yourusername/idempotency-gateway/middleware"
)

func main() {
	cfg := config.Load()
	store := database.NewStore(cfg.IdempotencyTTL)
	handler := &handlers.PaymentHandler{
		Store:           store,
		ProcessingDelay: cfg.ProcessingDelay,
	}

	mux := http.NewServeMux()
	mux.Handle("POST /process-payment", middleware.Idempotency(store)(http.HandlerFunc(handler.ProcessPayment)))

	log.Printf("Server starting on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
