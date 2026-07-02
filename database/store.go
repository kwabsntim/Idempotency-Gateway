package database

import (
	"sync"
	"time"

	"github.com/kwabsntim/idempotency-gateway/models"
)

// this is the main struct the holds the idempotency record
type Store struct {
	mu      sync.RWMutex
	records map[string]*models.IdempotencyRecord
	ttl     time.Duration
}

// constructor function for the Store struct, initializes the records map and sets the TTL for idempotency records
func NewStore(ttl time.Duration) *Store {
	return &Store{
		records: make(map[string]*models.IdempotencyRecord),
		ttl:     ttl,
	}
}

// this method or function retrives an idempotency record  from the store
// based on the provided key.
func (s *Store) Get(key string) (*models.IdempotencyRecord, bool) {
	s.mu.RLock()         //lock the store for reading
	defer s.mu.RUnlock() //release the lock after function execution
	record, ok := s.records[key]
	return record, ok
}

// function to set an idempotency record in the store with a given key.
func (s *Store) Set(key string, record *models.IdempotencyRecord) {
	s.mu.Lock()         //requires a write lock on the store
	defer s.mu.Unlock() //unlocks after function execution
	s.records[key] = record
}

// function to delete the idempotency from the store
func (s *Store) Delete(key string) {
	s.mu.Lock()         //requires a write lock on the store
	defer s.mu.Unlock() //unlocks after function execution
	delete(s.records, key)
}

// function to evict expired idempotency records from the store
func (s *Store) IsExpired(record *models.IdempotencyRecord) bool {
	return time.Since(record.CreatedAt) > s.ttl
}

// Unsafe variants — used by middleware which already holds the lock
func (s *Store) Lock()   { s.mu.Lock() }
func (s *Store) Unlock() { s.mu.Unlock() }

func (s *Store) GetUnsafe(key string) (*models.IdempotencyRecord, bool) {
	record, ok := s.records[key]
	return record, ok
}

func (s *Store) SetUnsafe(key string, record *models.IdempotencyRecord) {
	s.records[key] = record
}

func (s *Store) DeleteUnsafe(key string) {
	delete(s.records, key)
}
