package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yourusername/idempotency-gateway/database"
	"github.com/yourusername/idempotency-gateway/handlers"
	"github.com/yourusername/idempotency-gateway/models"
)

func Idempotency(store *database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Idempotency-Key header is required"})
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read request body"})
				return
			}
			hash := fmt.Sprintf("%x", sha256.Sum256(body))
			r.Body = io.NopCloser(bytes.NewReader(body))

			// Atomic check-and-create: hold the lock across Get + Set
			store.Lock()
			record, found := store.GetUnsafe(key)

			if found && !store.IsExpired(record) {
				if record.IsProcessing {
					store.Unlock()
					<-record.Done
					w.Header().Set("X-Cache-Hit", "true")
					writeJSON(w, record.StatusCode, record.Response)
					return
				}
				store.Unlock()
				if record.RequestHash != hash {
					writeJSON(w, http.StatusConflict, map[string]string{"error": "Idempotency key already used for a different request body."})
					return
				}
				w.Header().Set("X-Cache-Hit", "true")
				writeJSON(w, record.StatusCode, record.Response)
				return
			}

			if found {
				store.DeleteUnsafe(key)
			}

			newRecord := &models.IdempotencyRecord{
				RequestHash:  hash,
				IsProcessing: true,
				CreatedAt:    time.Now(),
				Done:         make(chan struct{}),
			}
			store.SetUnsafe(key, newRecord)
			store.Unlock()

			next.ServeHTTP(w, handlers.SetRecord(r, newRecord))
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
