// cmd/shortener/main.go
package main

import (
	"log"
	"net/http"

	"github.com/dkolesni-prog/transformer/internal/app"
	"github.com/go-chi/chi/v5"
)

var Version string = "iter5"

func main() {

	if err := run(); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}

// run sets up the router and starts the HTTP server.
func run() error {
	cfg := app.NewConfig()
	storage := app.NewStorage()

	r := chi.NewRouter()

	// Register endpoints
	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		app.ShortenURL(w, r, storage, cfg.BaseURL)
	})

	r.Get("/version/", func(w http.ResponseWriter, r *http.Request) {
		app.GetVersion(w, r, Version)
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		app.GetFullURL(w, r, storage)
	})

	log.Println("Running server on", cfg.RunAddr)
	return http.ListenAndServe(cfg.RunAddr, r)
}
