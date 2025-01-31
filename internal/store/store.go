// Internal/store/store.go.
package store

import (
	"context"
	"net/url"

	"github.com/dkolesni-prog/transformer/internal/config"
)

type Store interface {
	Save(ctx context.Context, url *url.URL, cfg *config.Config) (id string, err error)
	SaveBatch(ctx context.Context, urls []*url.URL, cfg *config.Config) ([]string, error)
	Load(ctx context.Context, id string) (url *url.URL, err error)
	Ping(ctx context.Context) error
	Close(ctx context.Context) error
}
