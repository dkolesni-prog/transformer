// Internal/store/store.go.
package store

import (
	"context"
	"net/url"

	"github.com/dkolesni-prog/transformer/internal/config"
)

type Store interface {
	Save(ctx context.Context, userID string, url *url.URL, cfg *config.Config) (id string, err error)
	SaveBatch(ctx context.Context, userID string, urls []*url.URL, cfg *config.Config) ([]string, error)
	Load(ctx context.Context, id string) (url *url.URL, err error)
	LoadUserURLs(ctx context.Context, userID string, baseURL string) ([]UserURL, error)
	Ping(ctx context.Context) error
	Close(ctx context.Context) error
}

type UserURL struct {
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}
