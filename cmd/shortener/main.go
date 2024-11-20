package main

import (
	"crypto/sha256"
	"encoding/hex"
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

	id := r.URL.Path[1:]

	long, exists := keyShortValueLong[id]
	if !exists {
		http.Error(w, "Short URL not found", http.StatusBadRequest)
		return
	}
	w.Header().Set("Location", long)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func main() {

	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {

	var keyLongValueShort = map[string]string{}
	var keyShortValueLong = map[string]string{}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/" {
			firstEndpoint(w, r, keyLongValueShort, keyShortValueLong)
		} else if r.Method == http.MethodGet && len(r.URL.Path) > 1 {
			secondEndpoint(w, r, keyShortValueLong)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	})

	return http.ListenAndServe(":8080", mux)
}
