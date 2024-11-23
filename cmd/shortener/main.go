package main

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
)

func firstEndpoint(w http.ResponseWriter,
	r *http.Request,
	keyLongValueShort map[string]string,
	keyShortValueLong map[string]string) {

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
	w.Write([]byte("http://localhost:8080/" + hashStr))
}

func secondEndpoint(w http.ResponseWriter,
	r *http.Request,
	keyShortValueLong map[string]string) {

	id := chi.URLParam(r, "id")

	long, exists := keyShortValueLong[id]
	if !exists {
		http.Error(w, "Short URL not found", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, long, http.StatusFound)
}

func main() {

	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {

	var keyLongValueShort = map[string]string{}
	var keyShortValueLong = map[string]string{}

	r := chi.NewRouter()

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		firstEndpoint(w, r, keyLongValueShort, keyShortValueLong)
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		secondEndpoint(w, r, keyShortValueLong)
	})

	return http.ListenAndServe(":8080", r)
}
