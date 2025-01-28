// Cmd/shortener/main.go
package main

import (
	"context"
	"errors"
	"github.com/dkolesni-prog/transformer/internal/app/endpoints"
	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"os"
	"os/signal"
	"syscall"

	"net/http"
	"time"

	"github.com/dkolesni-prog/transformer/internal/config"
	"github.com/dkolesni-prog/transformer/internal/helpers"
	"github.com/dkolesni-prog/transformer/internal/store"
)

const version = "iter14"

func main() {
	middleware.Initialize("info", version)
	if err := run(); err != nil {
		middleware.Log.Info().Err(err).Msg("Failed to run server")
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := config.NewConfig()
	middleware.InitAuth(cfg.SecretKey)

	storage, err := newStorage(ctx, cfg)
	if err != nil {
		middleware.Log.Error().Err(err).Msg("Could not connect to storage")
		return err
	}

	defer func() {
		if closeErr := storage.Close(ctx); closeErr != nil {
			middleware.Log.Error().Err(closeErr).Msg("Could not close context")
		}
	}()

	router := endpoints.NewRouter(cfg, storage, version)

	srv := &http.Server{
		Addr:    cfg.RunAddr,
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			middleware.Log.Error().Err(err).Msg("Server encountered an error")
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)

	sig := <-stop
	middleware.Log.Info().Msgf("Received signal %v. Shutting down the server...", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 1*time.Second) // was: 5*time.Second
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		middleware.Log.Error().Err(err).Msg("Server shutdown error")
		return err
	}

	middleware.Log.Info().Msg("Server exited cleanly")
	return nil

}

//nolint:unparam  // Retaining error return for bc if removed. the main is red
func newStorage(ctx context.Context, cfg *config.Config) (store.Store, error) {

	middleware.Log.Info().
		Str("address", cfg.RunAddr).
		Str("Running server on", cfg.BaseURL).
		Str("file_storage", cfg.FileStoragePath).
		Str("DB DSN is:", helpers.Classify(cfg.DatabaseDSN)).
		Msg("Initializing storage")

	if cfg.DatabaseDSN != "" {
		rdb, err := store.NewRDB(ctx, cfg.DatabaseDSN)
		if err == nil {
			bootErr := rdb.Bootstrap(ctx)
			if bootErr == nil {
				return rdb, nil
			}
			middleware.Log.Error().
				Err(bootErr).
				Msg("DB bootstrap error")
		} else {
			middleware.Log.Error().
				Err(err).
				Msg("NewRDB error")
		}
		middleware.Log.Warn().
			Msg("Falling back from DB to file/memory storage")
	}

	if cfg.FileStoragePath != "" {
		fileStore := store.NewStorage(cfg)
		return fileStore, nil
	}

	memoryStore := store.NewMemoryStorage()
	return memoryStore, nil
}
