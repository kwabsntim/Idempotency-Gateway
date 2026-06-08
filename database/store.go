package database

import (
	"sync"
	"time"

	"github.com/yourusername/idempotency-gateway/models"
)

// store struct to hold the idempotency records
type Store struct {
	mu      sync.RWMutex
	records map[string]*models.IdempotencyRecord
	ttl     time.Duration
}

// New store function to create a new record
func NewStore(ttl time.Duration) *Store {
	return &Store{
		records: make(map[string]*models.IdempotencyRecord),
		ttl:     ttl,
	}
}

// Get Record function to get the record from store
func (s *Store) Get(key string) (*models.IdempotencyRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[key]
	return record, ok
}

// set record function to set the record in store
func (s *Store) Set(key string, record *models.IdempotencyRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[key] = record
}

// clean up function to remove expired  records from the store
func (s *Store) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, key)
}

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
