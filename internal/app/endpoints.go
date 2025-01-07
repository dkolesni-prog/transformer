// Internal/app/endpoints.go.

package app

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

const errSomethingWentWrong = "Something went wrong"

func RandStringRunes(n int) string {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letterRunes))))
		if err != nil {
			Log.Printf("Error generating random number: %v", err)
			return ""
		}
		b[i] = letterRunes[num.Int64()]
	}
	return string(b)
}

func ensureTrailingSlash(rawURL string) string {
	if len(rawURL) == 0 {
		return rawURL
	}
	if rawURL[len(rawURL)-1] != '/' {
		return rawURL + "/"
	}
	return rawURL
}

func createShortURL(longURL string, storage *Storage, baseURL string) (string, error) { // mutual
	const maxRetries = 5
	const randValLength = 8
	var shortURL string
	var success bool

	for i := range make([]int, maxRetries) {
		randVal := RandStringRunes(randValLength)
		shortURL, success = storage.SetIfAbsent(randVal, longURL)
		if success {
			break
		}
		if i == maxRetries-1 {
			return "", errors.New("could not generate unique URL")
		}
	}

	fullShortURL := ensureTrailingSlash(baseURL) + shortURL
	return fullShortURL, nil
}

func ShortenURL(w http.ResponseWriter, r *http.Request, storage *Storage, baseURL string) {
	Log.Debug().Msg("Entered ShortenURL handler")
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		Log.Debug().Msg("Invalid method")
		return
	}

	body, err := io.ReadAll(r.Body) // read the entire request body as a raw string
	if err != nil {
		Log.Error().Err(err).Msg("Error reading request body")
		http.Error(w, errSomethingWentWrong, http.StatusInternalServerError)
		return
	}

	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			Log.Error().Err(cerr).Msg("Error closing request body")
		}
	}()

	longURL := string(body)
	Log.Debug().Msgf("Long URL: %s", longURL)
	if longURL == "" {
		Log.Error().Msg("Empty URL in request body")
		http.Error(w, errSomethingWentWrong, http.StatusInternalServerError)
		return
	}
	Log.Debug().Str("longURL", longURL).Msg("URL retrieved from raw body")

	parsedURL, err := url.ParseRequestURI(longURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		Log.Error().Err(err).Msgf("Invalid URL: %v", longURL)
		http.Error(w, errSomethingWentWrong, http.StatusInternalServerError)
		return
	}
	Log.Debug().Msg("URL validated successfully")

	shortURL, err := createShortURL(longURL, storage, baseURL)
	if err != nil {
		Log.Error().Err(err).Msg("Error creating short URL")
		http.Error(w, errSomethingWentWrong, http.StatusInternalServerError)
		return
	}
	Log.Debug().Str("shortURL", shortURL).Msg("Short URL created")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusCreated)

	if _, err := w.Write([]byte(shortURL)); err != nil {
		Log.Error().Err(err).Msg("Error writing response")
		http.Error(w, errSomethingWentWrong, http.StatusInternalServerError)
		return
	}

	Log.Debug().Msg("Short URL response sent")
}

func ShortenURLJSON(w http.ResponseWriter, r *http.Request, storage *Storage, baseURL string) {
	Log.Debug().Msg("Entered ShortenURLJSON handler")
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			Log.Error().Err(cerr).Msg("Error closing request body")
		}
	}()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}

	var reqData struct {
		URL string `json:"url"`
	}

	if err := json.Unmarshal(body, &reqData); err != nil {
		http.Error(w, "Failed to parse JSON", http.StatusBadRequest)
		return
	}

	if reqData.URL == "" {
		http.Error(w, "Empty url field in JSON", http.StatusBadRequest)
		return
	}

	parsedURL, err := url.ParseRequestURI(reqData.URL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	Log.Debug().Msgf("Long URL: %s", parsedURL)
	shortURL, err := createShortURL(reqData.URL, storage, baseURL)
	if err != nil {
		http.Error(w, "Failed to create short URL", http.StatusInternalServerError)
		return
	}

	responseData := map[string]string{
		"result": shortURL,
	}

	responseJSON, err := json.Marshal(responseData)
	if err != nil {
		http.Error(w, "Failed to marshal JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	if _, err := w.Write(responseJSON); err != nil {
		Log.Error().Err(err).Msg("Failed to write JSON response")
	}
}

func GetFullURL(w http.ResponseWriter, r *http.Request, storage *Storage) {
	id := chi.URLParam(r, "id")

	long, ok := storage.Get(id)
	if !ok {
		http.Error(w, "Short URL not found", http.StatusNotFound)
		Log.Printf("Could not find a short URL")
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
		http.Error(w, errSomethingWentWrong, http.StatusInternalServerError)
		Log.Printf("Error writing version response: %v", err)
		return
	}
}
