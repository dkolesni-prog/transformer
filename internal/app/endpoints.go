package app

import (
	"crypto/rand"
	"io"
	"log"
	"math/big"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func RandStringRunes(n int) string {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letterRunes))))
		if err != nil {
			panic(err)
		}
		b[i] = letterRunes[num.Int64()]
	}
	return string(b)
}

func ShortenURL(w http.ResponseWriter, r *http.Request, storage *Storage, baseURL string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("unable to read body of request: %v", err)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			log.Printf("Error closing request body: %v", err)
			http.Error(w, "Something went wrong", http.StatusInternalServerError)
		}
	}()

	const maxRetries = 5
	var randVal string

	const randValLength = 8
	for i := range maxRetries {
		randVal = RandStringRunes(randValLength)
		shortURL, success := storage.SetIfAbsent(randVal, string(body))
		if success {
			baseURL = ensureTrailingSlash(baseURL)
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusCreated)
			_, err = w.Write([]byte(baseURL + shortURL))
			if err != nil {
				log.Printf("Error writing response: %v", err)
				http.Error(w, "Something went wrong", http.StatusInternalServerError)
			}
			return
		}
		if i == maxRetries-1 {
			log.Printf("Could not generate unique URL")
			http.Error(w, "Something went wrong", http.StatusInternalServerError)
		}
	}

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
	_, err := w.Write([]byte(version))
	if err != nil {
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		log.Printf("Error writing version response: %v", err)
		return
	}
}
