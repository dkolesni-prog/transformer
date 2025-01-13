package endpoints

// Internal/app/endpoints/shortener.go
import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/dkolesni-prog/transformer/internal/app"
	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/dkolesni-prog/transformer/internal/store"
)

// ShortenURL handles a POST request with a raw long URL in the body.
func ShortenURL(w http.ResponseWriter, r *http.Request, s store.Store, cfg *app.Config) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		middleware.Log.Error().Err(err).Msg("Error reading request body")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	longURL := string(body)
	if longURL == "" {
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}

	parsedURL, err := url.ParseRequestURI(longURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// Let the store generate a short URL
	shortURL, err := s.Save(r.Context(), parsedURL, cfg)
	if err != nil {
		middleware.Log.Error().Err(err).Msg("Error creating short URL")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	if _, err := w.Write([]byte(shortURL)); err != nil {
		middleware.Log.Error().Err(err).Msg("Error writing response")
	}
}

// ShortenURLJSON handles a POST request with a JSON {"url":"..."} in the body.
func ShortenURLJSON(w http.ResponseWriter, r *http.Request, s store.Store, cfg *app.Config) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Failed to parse JSON", http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "Empty url field", http.StatusBadRequest)
		return
	}

	parsedURL, err := url.ParseRequestURI(req.URL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	shortURL, err := s.Save(r.Context(), parsedURL, cfg)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	response := map[string]string{"result": shortURL}
	respJSON, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Failed to marshal JSON", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	w.Write(respJSON)
}
