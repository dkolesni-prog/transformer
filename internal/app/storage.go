// Internal/app/Storage.go.

package app

import (
	"sync"
)

type Storage struct {
	mu                *sync.Mutex
	keyShortValuelong map[string]string
}

func NewStorage() *Storage {
	return &Storage{
		mu:                &sync.Mutex{},
		keyShortValuelong: make(map[string]string),
	}
}

func (s *Storage) SetIfAbsent(short, long string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.keyShortValuelong[short]; ok {
		return "", false
	}
	s.keyShortValuelong[short] = long
	return short, true
}

func (s *Storage) Get(short string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	long, ok := s.keyShortValuelong[short]
	return long, ok
}
