package app

import (
	"io"
	"math/rand"
	"net/http"

	"github.com/go-chi/chi/v5"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func ShortenURL(w http.ResponseWriter, r *http.Request, storage *Storage, baseURL string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Unable to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	randVal := RandStringRunes(8)
	storage.Set(randVal, string(body))
	baseURL = ensureTrailingSlash(baseURL)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(baseURL + randVal))
}

func GetFullURL(w http.ResponseWriter, r *http.Request, storage *Storage) {
	id := chi.URLParam(r, "id")

	long, ok := storage.Get(id)
	if !ok {
		http.Error(w, "Short URL not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, long, http.StatusTemporaryRedirect)
}

func GetVersion(w http.ResponseWriter, r *http.Request, version string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only use GET!", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(version))
}
