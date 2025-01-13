package main

import (
	"context"
	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/dkolesni-prog/transformer/internal/store"
	"net/http"
	"time"

	"github.com/dkolesni-prog/transformer/internal/app"
)

var Version = "iter10"

func main() {
	middleware.Initialize("info", Version)
	if err := run(); err != nil {
		middleware.Log.Info().Err(err).Msg("Failed to run server")
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	cfg := app.NewConfig()

	storage, err := newStorage(ctx, cfg)
	if err != nil {
		middleware.Log.Error().Err(err).Msg("Couldnt connect to storage")
	}
	defer storage.Close(ctx)

	router := app.NewRouter(ctx, cfg, storage, Version)

	return http.ListenAndServe(cfg.RunAddr, router)
}

func newStorage(ctx context.Context, cfg *app.Config) (store.Store, error) {
	middleware.Log.Info().
		Str("address", cfg.RunAddr).
		Str("Running server on", cfg.BaseURL).
		Str("file_storage", cfg.FileStoragePath).
		Str("database_dsn", cfg.DatabaseDSN).
		Msg("Initializing storage")

	if cfg.DatabaseDSN != "" {
		rdb, err := store.NewRDB(ctx, cfg.DatabaseDSN)
		if err == nil {
			bootErr := rdb.Bootstrap(ctx)
			if bootErr == nil {
				return rdb, nil
			}
			middleware.Log.Printf("DB bootstrap error: " + bootErr.Error())
		} else {
			middleware.Log.Printf("NewRDB error: " + err.Error())
		}
		middleware.Log.Printf("Falling back from DB to file/memory")
	}

	if cfg.FileStoragePath != "" {
		fileStore := store.NewStorage(cfg)
		return fileStore, nil
	}

	memoryStore := store.NewMemoryStorage()
	return memoryStore, nil
}
