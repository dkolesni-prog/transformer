// Internal/app/config/config.go.

package config

import (
	"flag"
	"os"
	"sync"

	"github.com/dkolesni-prog/transformer/internal/helpers"
)

type Config struct {
	RunAddr         string
	BaseURL         string
	FileStoragePath string
	DatabaseDSN     string
}

var parseOnce sync.Once

func NewConfig() *Config {
	cfg := Config{}

	parseOnce.Do(func() {
		flag.StringVar(&cfg.RunAddr, "a", ":8080", "address and port to run server")
		flag.StringVar(&cfg.BaseURL, "b", "http://localhost:8080/", "base URL for shortened links")
		flag.StringVar(&cfg.FileStoragePath, "f", "shortener_data.json", "path to file with shortener data")
		flag.StringVar(&cfg.DatabaseDSN, "d", "", "connection string to database")
		flag.Parse()
	})
	if envRunAddr, ok := os.LookupEnv("SERVER_ADDRESS"); ok {
		cfg.RunAddr = envRunAddr
	}
	if envBaseURL, ok := os.LookupEnv("BASE_URL"); ok {
		cfg.BaseURL = envBaseURL
	}
	if envFilePath, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		cfg.FileStoragePath = envFilePath
	}
	if envDatabaseDSN, ok := os.LookupEnv("DATABASE_DSN"); ok {
		cfg.DatabaseDSN = envDatabaseDSN
	}
	cfg.BaseURL = helpers.EnsureTrailingSlash(cfg.BaseURL)
	return &cfg
}
