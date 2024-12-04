package app

import (
	"sync"
)

type Storage struct {
	mu                sync.RWMutex
	keyShortValuelong map[string]string
}

func NewStorage() *Storage {
	return &Storage{
		keyShortValuelong: make(map[string]string),
	}
}

func (s *Storage) Set(short, long string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keyShortValuelong[short] = long
}

func (s *Storage) Get(short string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	long, ok := s.keyShortValuelong[short]
	return long, ok
}
