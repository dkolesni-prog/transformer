// internal/store/store.go
package store

import (
	"context"
	"net/url"

	"github.com/dkolesni-prog/transformer/internal/config"
)

// Store — обновлённый интерфейс.
// Вместо Load(...) теперь LoadFull(...) возвращает (URL, isDeleted, error).
type Store interface {
	Save(ctx context.Context, userID string, url *url.URL, cfg *config.Config) (string, error)
	SaveBatch(ctx context.Context, userID string, urls []*url.URL, cfg *config.Config) ([]string, error)
	LoadFull(ctx context.Context, shortID string) (*url.URL, bool, error)

	LoadUserURLs(ctx context.Context, userID string, baseURL string) ([]UserURL, error)
	DeleteBatch(ctx context.Context, userID string, shortIDs []string) error

	Ping(ctx context.Context) error
	Close(ctx context.Context) error
	Bootstrap(ctx context.Context) error
}

// UserURL — структура для вывода "своих" ссылок
type UserURL struct {
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}
