// internal/store/memory.go
package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/dkolesni-prog/transformer/internal/config"
	"github.com/dkolesni-prog/transformer/internal/helpers"
)

type MemoryRecord struct {
	OriginalURL string
	UserID      string
	IsDeleted   bool
}

type MemoryStorage struct {
	mu   sync.Mutex
	data map[string]MemoryRecord
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data: make(map[string]MemoryRecord),
	}
}

func (m *MemoryStorage) Bootstrap(ctx context.Context) error {
	return nil
}

func (m *MemoryStorage) Save(ctx context.Context, userID string, urlToSave *url.URL, cfg *config.Config) (string, error) {
	const maxRetries = 5
	const randLen = 8

	for i := 0; i < maxRetries; i++ {
		randVal, genErr := helpers.RandStringRunes(randLen)
		if genErr != nil {
			return "", fmt.Errorf("randVal: %w", genErr)
		}
		m.mu.Lock()
		_, exists := m.data[randVal]
		if !exists {
			m.data[randVal] = MemoryRecord{
				OriginalURL: urlToSave.String(),
				UserID:      userID,
				IsDeleted:   false,
			}
			m.mu.Unlock()
			return ensureSlash(cfg.BaseURL) + randVal, nil
		}
		m.mu.Unlock()
	}
	return "", errors.New("could not generate unique short ID")
}

func (m *MemoryStorage) SaveBatch(ctx context.Context, userID string, urls []*url.URL, cfg *config.Config) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []string
	for _, u := range urls {
		key := fmt.Sprintf("%x", len(m.data))
		m.data[key] = MemoryRecord{
			OriginalURL: u.String(),
			UserID:      userID,
			IsDeleted:   false,
		}
		out = append(out, ensureSlash(cfg.BaseURL)+key)
	}
	return out, nil
}

func (m *MemoryStorage) LoadFull(ctx context.Context, shortID string) (*url.URL, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rec, ok := m.data[shortID]
	if !ok {
		return nil, false, errors.New("not found")
	}
	parsed, err := url.Parse(rec.OriginalURL)
	if err != nil {
		return nil, false, errors.New("invalid stored URL")
	}
	return parsed, rec.IsDeleted, nil
}

func (m *MemoryStorage) LoadUserURLs(ctx context.Context, userID string, baseURL string) ([]UserURL, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var res []UserURL
	for shortID, rec := range m.data {
		if rec.UserID == userID && !rec.IsDeleted {
			res = append(res, UserURL{
				ShortURL:    ensureSlash(baseURL) + shortID,
				OriginalURL: rec.OriginalURL,
			})
		}
	}
	return res, nil
}

func (m *MemoryStorage) DeleteBatch(ctx context.Context, userID string, shortIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sid := range shortIDs {
		rec, ok := m.data[sid]
		if !ok {
			continue
		}
		if rec.UserID == userID {
			rec.IsDeleted = true
			m.data[sid] = rec
		}
	}
	return nil
}

func (m *MemoryStorage) Ping(ctx context.Context) error {
	return nil
}

func (m *MemoryStorage) Close(ctx context.Context) error {
	return nil
}
