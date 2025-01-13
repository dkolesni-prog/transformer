package store

import (
	"context"
	"errors"
	"net/url"
	"sync"

	"github.com/dkolesni-prog/transformer/internal/app"
	"github.com/dkolesni-prog/transformer/internal/app/endpoints"
)

type MemoryStorage struct {
	mu   sync.Mutex
	data map[string]string // short -> original
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data: make(map[string]string),
	}
}

func (m *MemoryStorage) Save(ctx context.Context, urlToSave *url.URL, cfg *app.Config) (string, error) {
	const maxRetries = 5
	const randValLength = 8

	for i := 0; i < maxRetries; i++ {
		randVal, err := endpoints.RandStringRunes(randValLength)
		if err != nil {
			return "", errors.New("could not generate random string")
		}

		short, success := m.setIfAbsent(randVal, urlToSave.String())
		if success {
			fullShort := endpoints.EnsureTrailingSlash(cfg.BaseURL) + short
			return fullShort, nil
		}
		if i == maxRetries-1 {
			return "", errors.New("could not generate unique short ID")
		}
	}
	return "", errors.New("unexpected error generating short ID")
}

func (m *MemoryStorage) SaveBatch(ctx context.Context, urls []*url.URL, cfg *app.Config) ([]string, error) {
	if len(urls) == 0 {
		return nil, nil
	}
	results := make([]string, 0, len(urls))
	for _, u := range urls {
		short, err := m.Save(ctx, u, cfg)
		if err != nil {
			return nil, err
		}
		results = append(results, short)
	}
	return results, nil
}

func (m *MemoryStorage) Load(ctx context.Context, id string) (*url.URL, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	longVal, ok := m.data[id]
	if !ok {
		return nil, errors.New("short ID not found")
	}
	parsed, err := url.Parse(longVal)
	if err != nil {
		return nil, errors.New("invalid stored URL")
	}
	return parsed, nil
}

func (m *MemoryStorage) Ping(ctx context.Context) error {
	return nil
}

func (m *MemoryStorage) Close(ctx context.Context) error {
	return nil
}

func (m *MemoryStorage) Bootstrap(ctx context.Context) error {
	return nil
}

func (m *MemoryStorage) setIfAbsent(short, original string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.data[short]; exists {
		return "", false
	}
	m.data[short] = original
	return short, true
}
