package main

import (
	"flag"
)

type Config struct {
	RunAddr string
	BaseURL string
}

func NewConfig() *Config {
	cfg := Config{}
	flag.StringVar(&cfg.RunAddr, "a", ":8080", "address and port to run server")
	flag.StringVar(&cfg.BaseURL, "b", "http://localhost:8080/", "base URL for shortened links")
	flag.Parse()
	return &cfg
}
