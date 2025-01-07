package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/dkolesni-prog/transformer/internal/app"
)

var Version string = "iter5"

func main() {
	if err := run(); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}

// run sets up the configuration, storage, router, and starts the HTTP server.
func run() error {
	cfg := app.NewConfig()
	storage := app.NewStorage()
	router := app.NewRouter(cfg, storage, Version)

	log.Println("Running server on", cfg.RunAddr)
	if err := http.ListenAndServe(cfg.RunAddr, router); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}
