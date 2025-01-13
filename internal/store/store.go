// Internal/store/store.go
package store

import (
	"context"
	"github.com/dkolesni-prog/transformer/internal/app"
	"net/url"
)

type Store interface {
	Save(ctx context.Context, url *url.URL, cfg *app.Config) (id string, err error)
	SaveBatch(ctx context.Context, urls []*url.URL, cfg *app.Config) ([]string, error)
	Load(ctx context.Context, id string) (url *url.URL, err error)
	Ping(ctx context.Context) error
	Close(ctx context.Context) error
}
