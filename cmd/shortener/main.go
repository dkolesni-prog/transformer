package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
)

func firstEndpoint(w http.ResponseWriter,
	r *http.Request,
	keyLongValueShort map[string]string,
	keyShortValueLong map[string]string,
	cfg *Config) {

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Unable to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	hash := sha256.Sum256(body)
	hashStr := hex.EncodeToString(hash[:])[:8]
	keyLongValueShort[string(body)] = hashStr
	keyShortValueLong[hashStr] = string(body)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(201)
	w.Write([]byte(cfg.BaseURL + hashStr))
}

func secondEndpoint(w http.ResponseWriter,
	r *http.Request,
	keyShortValueLong map[string]string) {

	id := chi.URLParam(r, "id")

	long, exists := keyShortValueLong[id]
	if !exists {
		http.Error(w, "Short URL not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, long, http.StatusTemporaryRedirect)
}

func main() {

	cfg := NewConfig()
	if err := run(cfg); err != nil {
		panic(err)
	}
}

func run(cfg *Config) error {

	var keyLongValueShort = map[string]string{}
	var keyShortValueLong = map[string]string{}

	r := chi.NewRouter()

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		firstEndpoint(w, r, keyLongValueShort, keyShortValueLong, cfg)
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		secondEndpoint(w, r, keyShortValueLong)
	})

	fmt.Println("Running server on", cfg.RunAddr)
	return http.ListenAndServe(cfg.RunAddr, r)
}
