package main

import (
	"net/http"

	"github.com/dkolesni-prog/transformer/internal/app"
)

var Version string = "iter7"

// gzipMiddleware handles gzip compression/decompression for requests and responses.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decompress gzipped request body
		if r.Header.Get("Content-Encoding") == "gzip" {
			cr, err := newCompressReader(r.Body)
			if err != nil {
				http.Error(w, "Failed to decode gzip", http.StatusBadRequest)
				return
			}
			defer cr.Close()

			r.Body = cr
			r.Header.Del("Content-Encoding")
		}

		// Compress responses if client supports gzip
		if r.Header.Get("Accept-Encoding") == "gzip" {
			cw := newCompressWriter(w)
			defer cw.Close()
			w = cw
			w.Header().Set("Content-Encoding", "gzip")
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	app.Initialize("info", Version)
	if err := run(); err != nil {
		app.Log.Info().Err(err).Msg("Failed to run server")
	}
}

func run() error {
	cfg := app.NewConfig()

	storage := app.NewStorage(cfg.FileStoragePath)

	router := app.NewRouter(cfg, storage, Version)

	app.Log.Info().
		Str("address", cfg.RunAddr).
		Str("file_storage", cfg.FileStoragePath).
		Msg("Running server on")

	return http.ListenAndServe(cfg.RunAddr, gzipMiddleware(app.WithLogging(router)))
}
