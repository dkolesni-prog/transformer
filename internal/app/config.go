// Internal/app/config.go.
package app

import (
	"flag"
	"os"
)

type Config struct {
	RunAddr string
	BaseURL string
}

func ensureTrailingSlash(url string) string {
	if len(url) > 0 && url[len(url)-1] != '/' {
		return url + "/"
	}
	return url
}

func NewConfig() *Config {
	cfg := Config{}
	flag.StringVar(&cfg.RunAddr, "a", ":8080", "address and port to run server")
	flag.StringVar(&cfg.BaseURL, "b", "http://localhost:8080/", "base URL for shortened links")
	flag.Parse()

	if envRunAddr, ok := os.LookupEnv("SERVER_ADDRESS"); ok {
		cfg.RunAddr = envRunAddr
	}

	if envBaseURL, ok := os.LookupEnv("BASE_URL"); ok {
		cfg.BaseURL = envBaseURL
	}

	cfg.BaseURL = ensureTrailingSlash(cfg.BaseURL)

	return &cfg
}
