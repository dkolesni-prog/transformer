// Cmd/shortener/main.go.

package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/dkolesni-prog/transformer/internal/app"
)

var Version string = "iter7"

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			cr, err := newCompressReader(r.Body)
			if err != nil {
				app.Log.Error().Err(err).Msg("Failed to create gzip reader")
				http.Error(w, "Failed to decode gzip", http.StatusBadRequest)
				return
			}
			defer cr.Close()
			r.Body = cr
		}

		ow := w
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			cw := newCompressWriter(w)
			ow = cw
			defer func() {
				if cerr := cw.Close(); cerr != nil {
					app.Log.Error().Err(cerr).Msg("Failed to close gzip writer")
				}
			}()
		}
		next.ServeHTTP(ow, r)
	})
}

func main() {
	app.Initialize("info", Version)
	if err := run(); err != nil {
		app.Log.Info().
			Err(err).
			Msg("Failed to run server")
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

	if err := http.ListenAndServe(cfg.RunAddr, app.WithLogging(gzipMiddleware(router))); err != nil {
		app.Log.Info().
			Err(err).
			Msg("Failed to start server")
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}
