package database

import (
	"sync"
	"time"

	"github.com/yourusername/idempotency-gateway/models"
)

//store struct to hold the idempotency records 
type Store struct{
	mu sync.RWMutex
	records map[string]*models.IdempotencyRecord
	ttl time.Duration
}

//New store function to create a new record 
func Newstore(ttl time.Duration)* Store{
	return &Store{
		records: make(map[string]*models.IdempotencyRecord),
		ttl: ttl,
	}
}

//Get Record function to get the record from store 
func (s *Store)GetStore(Key string)(*models.IdempotencyRecord,bool){
	s.mu.RLock()//this sets a read lock on the store.
	defer s.mu.RUnlock()
	record,ok:=s.records[Key]
	return record,ok
}
//set record function to set the record in store
func(s *Store)Set(Key string,record *models.IdempotencyRecord){
	s.mu.Lock()
	defer s.mu
}