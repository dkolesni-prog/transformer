package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(cfg *Config, storage *Storage, version string) http.Handler {
	r := chi.NewRouter()

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		ShortenURL(w, r, storage, cfg.BaseURL)
	})

	r.Get("/version/", func(w http.ResponseWriter, r *http.Request) {
		GetVersion(w, r, version)
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		GetFullURL(w, r, storage)
	})

	return r
}
