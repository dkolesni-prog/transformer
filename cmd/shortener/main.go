package main

import (
	"net/http"

	"github.com/dkolesni-prog/transformer/internal/app"
)

var Version string = "iter7"

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

	return http.ListenAndServe(cfg.RunAddr, app.GzipMiddleware(app.WithLogging(router)))
}
