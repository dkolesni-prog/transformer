// internal/app/endpoints/endpoints.go
package endpoints

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/dkolesni-prog/transformer/internal/config"
	"github.com/dkolesni-prog/transformer/internal/store"

	"github.com/go-chi/chi/v5"
)

const (
	internalServerError = "Internal Server Error."
	contentType         = "Content-Type"
	contentTypeJSON     = "application/json; charset=utf-8"
	contentTypeText     = "text/plain; charset=utf-8"
)

func NewRouter(cfg *config.Config, s store.Store, version string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.WithLogging, middleware.GzipMiddleware)
	r.Use(middleware.AuthMiddleware)

	// Сокращение URL
	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		ShortenURL(w, r, s, cfg)
	})

	r.Post("/api/shorten", func(w http.ResponseWriter, r *http.Request) {
		ShortenURLJSON(w, r, s, cfg)
	})

	// Batch
	r.Post("/api/shorten/batch", func(w http.ResponseWriter, r *http.Request) {
		ShortenBatch(w, r, s, cfg)
	})

	// Удаление (DELETE /api/user/urls)
	r.Delete("/api/user/urls", func(w http.ResponseWriter, r *http.Request) {
		DeleteUserURLs(w, r, s)
	})

	// Список "своих" ссылок
	r.Get("/api/user/urls", func(w http.ResponseWriter, r *http.Request) {
		GetUserURLs(w, r, s, cfg)
	})

	// GET /{id}
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		GetFullURL(w, r, s)
	})

	// Ping
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		Ping(w, r, s)
	})

	// Версия
	r.Get("/version/", func(w http.ResponseWriter, r *http.Request) {
		GetVersion(w, r, version)
	})

	return r
}

// DeleteUserURLs удаляет ссылки
func DeleteUserURLs(w http.ResponseWriter, r *http.Request, s store.Store) {
	userIDAny := r.Context().Value("userID")
	userID, ok := userIDAny.(string)
	if !ok || userID == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "unauthorized",
		})
		return
	}

	// Считываем массив shortIDs
	var toDelete []string
	if err := json.NewDecoder(r.Body).Decode(&toDelete); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Асинхронно помечаем ссылки удалёнными:
	go func() {
		err := s.DeleteBatch(r.Context(), userID, toDelete)
		if err != nil {
			middleware.Log.Error().Err(err).
				Strs("shortIDs", toDelete).
				Msg("Failed to mark URLs as deleted")
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

func GetUserURLs(w http.ResponseWriter, r *http.Request, s store.Store, cfg *config.Config) {

	userID, ok := middleware.GetUserID(r)

	if !ok || userID == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "unauthorized",
		})
		return
	}

	list, err := s.LoadUserURLs(r.Context(), userID, cfg.BaseURL)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if len(list) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set(contentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(list); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// GetFullURL — проверка удалён/не удалён
func GetFullURL(w http.ResponseWriter, r *http.Request, s store.Store) {
	id := chi.URLParam(r, "id")

	longURL, isDeleted, err := s.LoadFull(r.Context(), id)
	if err != nil {
		http.Error(w, "Short URL not found", http.StatusNotFound)
		return
	}
	if isDeleted {
		http.Error(w, "URL is gone", http.StatusGone)
		return
	}
	http.Redirect(w, r, longURL.String(), http.StatusTemporaryRedirect)
}

func ShortenBatch(w http.ResponseWriter, r *http.Request, s store.Store, cfg *config.Config) {
	type BatchRequestItem struct {
		CorrelationID string `json:"correlation_id"`
		OriginalURL   string `json:"original_url"`
	}
	type BatchResponseItem struct {
		CorrelationID string `json:"correlation_id"`
		ShortURL      string `json:"short_url"`
	}

	var reqs []BatchRequestItem
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if len(reqs) == 0 {
		http.Error(w, "Empty batch", http.StatusBadRequest)
		return
	}

	var urls []*url.URL
	corrMap := make(map[*url.URL]string)
	for _, rItem := range reqs {
		parsed, pErr := url.ParseRequestURI(rItem.OriginalURL)
		if pErr != nil {
			http.Error(w, "Invalid URL in batch", http.StatusBadRequest)
			return
		}
		urls = append(urls, parsed)
		corrMap[parsed] = rItem.CorrelationID
	}
	userID, _ := r.Context().Value("userID").(string)
	shorts, err := s.SaveBatch(r.Context(), userID, urls, cfg)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	var resp []BatchResponseItem
	for i, shortU := range shorts {
		resp = append(resp, BatchResponseItem{
			CorrelationID: corrMap[urls[i]],
			ShortURL:      shortU,
		})
	}

	w.Header().Set(contentType, contentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func ShortenURL(w http.ResponseWriter, r *http.Request, s store.Store, cfg *config.Config) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	longURL := string(body)
	if longURL == "" {
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}
	parsed, pErr := url.ParseRequestURI(longURL)
	if pErr != nil || parsed.Scheme == "" || parsed.Host == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	userID, _ := r.Context().Value("userID").(string)
	res, saveErr := s.Save(r.Context(), userID, parsed, cfg)
	if saveErr != nil {
		if strings.Contains(saveErr.Error(), "conflict") {
			w.Header().Set(contentType, contentTypeText)
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(res))
			return
		}
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentType, contentTypeText)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(res))
}

func ShortenURLJSON(w http.ResponseWriter, r *http.Request, s store.Store, cfg *config.Config) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

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

	parsed, pErr := url.ParseRequestURI(req.URL)
	if pErr != nil || parsed.Scheme == "" || parsed.Host == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	userID, _ := r.Context().Value("userID").(string)
	shortU, saveErr := s.Save(r.Context(), userID, parsed, cfg)
	if saveErr != nil {
		if strings.Contains(saveErr.Error(), "conflict") {
			w.Header().Set(contentType, contentTypeJSON)
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"result": shortU})
			return
		}
		http.Error(w, internalServerError, http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentType, contentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"result": shortU})
}

func Ping(w http.ResponseWriter, r *http.Request, s store.Store) {
	if err := s.Ping(r.Context()); err != nil {
		http.Error(w, "DB connection failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func GetVersion(w http.ResponseWriter, r *http.Request, version string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only use GET!", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set(contentType, "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(version))
}
