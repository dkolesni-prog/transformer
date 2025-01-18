package endpoints

// Internal/app/endpoints/endpoints.go.
import (
	"context"
	"net/http"

	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/dkolesni-prog/transformer/internal/config"
	"github.com/dkolesni-prog/transformer/internal/store"

	"encoding/json"
	"io"
	"net/url"

	"github.com/go-chi/chi/v5"
)

const errSomethingWentWrong = "Something went wrong"
const internalServerError = "Internal Server Error"

func NewRouter(ctx context.Context, cfg *config.Config, s store.Store, version string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.GzipMiddleware, middleware.WithLogging)

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		ShortenURL(w, r, s, cfg)
	})

	r.Post("/api/shorten", func(w http.ResponseWriter, r *http.Request) {
		ShortenURLJSON(w, r, s, cfg)
	})

	r.Get("/version/", func(w http.ResponseWriter, r *http.Request) {
		GetVersion(w, r, version)
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		GetFullURL(ctx, w, r, s)
	})

	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		Ping(w, r, s)
	})

	// r.Post("/api/shorten/batch", func(w http.ResponseWriter, r *http.Request) {
	//	ShortenBatch(w, r, s, cfg)
	// })

	return r
}

// ShortenURL handles a POST request with a raw long URL in the body.
func ShortenURL(w http.ResponseWriter, r *http.Request, s store.Store, cfg *config.Config) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		middleware.Log.Error().Err(err).Msg("Error reading request body")
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}

	defer func() {
		if err := r.Body.Close(); err != nil {
			middleware.Log.Error().Err(err).Msg("Error closing request body")
		}
	}()

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
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	if _, err := w.Write([]byte(shortURL)); err != nil {
		middleware.Log.Error().Err(err).Msg("Error writing response")
	}
}

// ShortenURLJSON handles a POST request with a JSON {"url":"..."} in the body.
func ShortenURLJSON(w http.ResponseWriter, r *http.Request, s store.Store, cfg *config.Config) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	defer func() {
		if err := r.Body.Close(); err != nil {
			middleware.Log.Error().Err(err).Msg("Error closing request body")
		}
	}()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, internalServerError, http.StatusInternalServerError)
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
		http.Error(w, internalServerError, http.StatusInternalServerError)
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
	_, err = w.Write(respJSON)
	if err != nil {
		middleware.Log.Error().Err(err).Msg("Error writing")
	}
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
		middleware.Log.Printf("Error writing version response: %v", err)
		return
	}
}

func GetFullURL(ctx context.Context, w http.ResponseWriter, r *http.Request, s store.Store) {
	id := chi.URLParam(r, "id")

	long, ok := s.Load(ctx, id)
	if ok != nil {
		http.Error(w, "Short URL not found", http.StatusNotFound)
		middleware.Log.Printf("Could not find a short URL")
		return
	}
	http.Redirect(w, r, long.String(), http.StatusTemporaryRedirect)
}

func Ping(w http.ResponseWriter, r *http.Request, s store.Store) {
	if err := s.Ping(r.Context()); err != nil {
		http.Error(w, "DB connection failed", http.StatusInternalServerError)
		return
	}
	// Otherwise, return 200 OK
	w.WriteHeader(http.StatusOK)
}
