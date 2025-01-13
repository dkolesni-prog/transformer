// Internal/app/router.go.

package app

import (
	"context"
	"github.com/dkolesni-prog/transformer/internal/app/endpoints"
	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/dkolesni-prog/transformer/internal/store"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(ctx context.Context, cfg *Config, s store.Store, version string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.GzipMiddleware, middleware.WithLogging)

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		endpoints.ShortenURL(w, r, s, cfg)
	})

	r.Get("/version/", func(w http.ResponseWriter, r *http.Request) {
		endpoints.GetVersion(w, r, version)
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		endpoints.GetFullURL(ctx, w, r, s)
	})

	r.Post("/api/shorten", func(w http.ResponseWriter, r *http.Request) {
		endpoints.ShortenURLJSON(w, r, s, cfg)
	})

	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		endpoints.Ping(w, r, s)
	})

	r.Post("/api/shorten/batch", func(w http.ResponseWriter, r *http.Request) {
		endpoints.ShortenBatch(w, r, s, cfg)
	})

	return r
}
