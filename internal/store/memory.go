// Internal/store/memory.go.

package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/dkolesni-prog/transformer/internal/config"
	"github.com/dkolesni-prog/transformer/internal/helpers"
	"net/url"
	"sync"
)

type MemoryRecord struct {
	OriginalURL string
	UserID      string
}

type MemoryStorage struct {
	mu   sync.Mutex
	data map[string]MemoryRecord // shortID -> MemoryRecord
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data: make(map[string]MemoryRecord),
	}
}

func (m *MemoryStorage) Save(ctx context.Context, userID string, urlToSave *url.URL, cfg *config.Config) (string, error) {
	const maxRetries = 5
	const randLen = 8

	for i := 0; i < maxRetries; i++ {
		randVal, err := helpers.RandStringRunes(randLen)
		if err != nil {
			return "", err
		}

		m.mu.Lock()
		_, exists := m.data[randVal]
		if !exists {
			m.data[randVal] = MemoryRecord{
				OriginalURL: urlToSave.String(),
				UserID:      userID,
			}
			m.mu.Unlock()
			// Возвращаем
			return ensureSlash(cfg.BaseURL) + randVal, nil
		}
		m.mu.Unlock()
	}
	return "", errors.New("could not generate unique short ID")
}

func (m *MemoryStorage) SaveBatch(ctx context.Context, userID string, urls []*url.URL, cfg *config.Config) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, 0, len(urls))
	for _, u := range urls {
		id := fmt.Sprintf("%x", len(m.data))
		m.data[id] = MemoryRecord{
			OriginalURL: u.String(),
			UserID:      userID,
		}
		result = append(result, ensureSlash(cfg.BaseURL)+id)
	}
	return result, nil
}

func (m *MemoryStorage) LoadUserURLs(ctx context.Context, userID string, baseURL string) ([]UserURL, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []UserURL
	for shortID, rec := range m.data {
		if rec.UserID == userID {
			list = append(list, UserURL{
				ShortURL:    ensureSlash(baseURL) + shortID,
				OriginalURL: rec.OriginalURL,
			})
		}
	}
	return list, nil
}

func (m *MemoryStorage) Load(ctx context.Context, id string) (*url.URL, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rec, ok := m.data[id]
	if !ok {
		return nil, errors.New("short ID not found")
	}
	parsed, err := url.Parse(rec.OriginalURL)
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
	m.data[short] = MemoryRecord{
		OriginalURL: original,
		UserID:      "",
	}
	return short, true
}
