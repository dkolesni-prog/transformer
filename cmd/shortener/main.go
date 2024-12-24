// Cmd/shortener/main.go.

package main

import (
	"fmt"
	"net/http"

	"github.com/dkolesni-prog/transformer/internal/app"
)

var Version string = "iter7"

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
	storage := app.NewStorage()
	router := app.NewRouter(cfg, storage, Version)

	app.Log.Info().
		Str("address", cfg.RunAddr).
		Msg("Running server on")

	if err := http.ListenAndServe(cfg.RunAddr, app.WithLogging(router)); err != nil {
		app.Log.Info().
			Err(err).
			Msg("Failed to start server")
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}
