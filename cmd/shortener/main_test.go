package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dkolesni-prog/transformer/internal/app/endpoints"
	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/dkolesni-prog/transformer/internal/config"
	"github.com/dkolesni-prog/transformer/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEndpoints tests the main endpoints of the URL shortening service.
func TestEndpoints(t *testing.T) {
	// Update with a valid file path for the test
	cfg := config.NewConfig()
	storage := store.NewStorage(cfg)

	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		setup      func(*store.Storage)
		wantCode   int
		wantBody   string
		wantHeader map[string]string
	}{
		{
			name:     "POST valid URL",
			method:   http.MethodPost,
			url:      "/",
			body:     "https://example.com",
			setup:    func(s *store.Storage) {},
			wantCode: http.StatusCreated,
			wantBody: cfg.BaseURL,
			wantHeader: map[string]string{
				"Content-Type": "text/plain; charset=utf-8",
			},
		},
		{
			name:   "GET valid short URL",
			method: http.MethodGet,
			url:    "/abcd1234",
			body:   "",
			setup: func(s *store.Storage) {
				s.SetIfAbsent("abcd1234", "https://example.com")
			},
			wantCode: http.StatusTemporaryRedirect,
			wantBody: "",
			wantHeader: map[string]string{
				"Location": "https://example.com",
			},
		},
		{
			name:     "GET nonexistent short URL",
			method:   http.MethodGet,
			url:      "/nonexistent",
			body:     "",
			setup:    func(s *store.Storage) {},
			wantCode: http.StatusNotFound,
			wantBody: "Short URL not found\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(storage)
			}

			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.url, strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, tt.url, http.NoBody)
			}
			rec := httptest.NewRecorder()

			r := chi.NewRouter()
			r.Post("/", func(w http.ResponseWriter, r *http.Request) {
				endpoints.ShortenURL(w, r, storage, cfg)
			})
			r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
				endpoints.GetFullURL(context.Background(), w, r, storage)
			})

			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("got status code %d, want %d", rec.Code, tt.wantCode)
			}

			if tt.wantBody != "" && !strings.HasPrefix(rec.Body.String(), tt.wantBody) {
				t.Errorf("got body %q, want prefix %q", rec.Body.String(), tt.wantBody)
			}

			for key, wantValue := range tt.wantHeader {
				gotValue := rec.Header().Get(key)
				if gotValue != wantValue {
					t.Errorf("got header %q=%q, want %q=%q", key, gotValue, key, wantValue)
				}
			}
		})
	}
}

// TestGzipHandling checks gzip request/response support and content-type handling.
func TestGzipHandling(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		defer func() {
			if err := r.Body.Close(); err != nil {
				log.Println("Could not close request body:", err)
			}
		}()

		switch {
		case strings.Contains(string(body), "json"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"message": "JSON response"}`)); err != nil {
				return
			}
		case strings.Contains(string(body), "html"):
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte("<html><body>HTML response</body></html>")); err != nil {
				return
			}
		default:
			http.Error(w, "Unsupported content", http.StatusBadRequest)
		}
	})
	ts := httptest.NewServer(middleware.GzipMiddleware(router))
	defer ts.Close()

	t.Run("Accept gzip-encoded request", func(t *testing.T) {
		body := `{"content":"json"}`
		var gzippedBody bytes.Buffer
		gz := gzip.NewWriter(&gzippedBody)
		_, err := gz.Write([]byte(body))
		require.NoError(t, err)
		require.NoError(t, gz.Close())

		req, _ := http.NewRequest(http.MethodPost, ts.URL, &gzippedBody)
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Println("Could not close response body:", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Serve gzip-encoded response", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(`{"content":"html"}`))
		req.Header.Set("Content-Type", "text/html")
		req.Header.Set("Accept-Encoding", "gzip")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Println("Could not close response body:", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

		gzr, err := gzip.NewReader(resp.Body)
		require.NoError(t, err)

		defer func() {
			if err := gzr.Close(); err != nil {
				log.Println("Could not close gzip reader body:", err)
			}
		}()

		body, err := io.ReadAll(gzr)
		require.NoError(t, err)
		assert.Equal(t, "<html><body>HTML response</body></html>", string(body))
	})

	t.Run("Unsupported content type", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(`{"content":"unknown"}`))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Println("Could not close response body:", err)
			}
		}()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
