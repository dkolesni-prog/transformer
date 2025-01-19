package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/require"

	"github.com/dkolesni-prog/transformer/internal/app/endpoints"
	"github.com/dkolesni-prog/transformer/internal/config"
	"github.com/dkolesni-prog/transformer/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
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
			name:   "POST batch of URLs",
			method: http.MethodPost,
			url:    "/api/shorten/batch",
			body: `[
		{"correlation_id":"1", "original_url":"https://example1.com"}, 
		{"correlation_id":"2", "original_url":"https://example2.com"}
	]`,
			setup:    func(s *store.Storage) {},
			wantCode: http.StatusCreated,
			wantBody: `[{"correlation_id":"1", "short_url":"`, // Verify that it contains the expected prefix
			wantHeader: map[string]string{
				"Content-Type": "application/json; charset=utf-8",
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

			r.Post("/api/shorten/batch", func(w http.ResponseWriter, r *http.Request) {
				endpoints.ShortenBatch(w, r, storage, cfg)
			})

			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("got status code %d, want %d", rec.Code, tt.wantCode)
			}

			if tt.method == http.MethodPost && strings.Contains(tt.url, "/batch") {
				var results []map[string]string
				err := json.Unmarshal(rec.Body.Bytes(), &results)
				require.NoError(t, err, "Failed to unmarshal batch response")

				for _, result := range results {
					assert.Contains(t, result["short_url"], cfg.BaseURL, "Short URL should start with base URL")
				}
			} else if tt.wantBody != "" && !strings.HasPrefix(rec.Body.String(), tt.wantBody) {
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

func TestGzipHandling(t *testing.T) {
	// Initialize configuration
	cfg := config.NewConfig()
	storeNotImported := store.NewMemoryStorage()

	// Initialize the real router with endpoints and middleware
	router := endpoints.NewRouter(context.Background(), cfg, storeNotImported, "testversion")
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Initialize Resty client
	client := resty.New().
		SetBaseURL(ts.URL).
		SetRedirectPolicy(resty.NoRedirectPolicy())

	// Test case: Accept gzip-encoded request
	t.Run("Accept gzip-encoded request", func(t *testing.T) {
		body := `{"content":"json"}`
		var gzippedBody bytes.Buffer
		gz := gzip.NewWriter(&gzippedBody)
		_, err := gz.Write([]byte(body))
		require.NoError(t, err)
		require.NoError(t, gz.Close())

		resp, err := client.R().
			SetHeader("Content-Encoding", "gzip").
			SetHeader("Content-Type", "application/json").
			SetBody(gzippedBody.Bytes()).
			Post("/api/shorten")
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode(), "Expected status code 201 Created")
	})

	// Test case: Serve gzip-encoded response
	t.Run("Serve gzip-encoded response", func(t *testing.T) {
		body := `{"url":"https://example.com"}`
		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Accept-Encoding", "gzip").
			SetBody(body).
			Post("/api/shorten")
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode(), "Expected status code 201 Created")
		assert.Contains(t, resp.Header().Get("Content-Encoding"), "gzip", "Expected Content-Encoding to be gzip")

		gzr, err := gzip.NewReader(bytes.NewReader(resp.Body()))
		require.NoError(t, err)
		defer func(gzr *gzip.Reader) {
			err := gzr.Close()
			if err != nil {
				middleware.Log.Error().Msg("Couldnt close gzip reader")
			}
		}(gzr)

		decompressedBody, err := io.ReadAll(gzr)
		require.NoError(t, err)

		var respData map[string]string
		err = json.Unmarshal(decompressedBody, &respData)
		require.NoError(t, err)
		assert.Contains(t, respData["result"], cfg.BaseURL, "Short URL should start with base URL")
	})

	// Test case: Unsupported content type
	t.Run("Unsupported content type", func(t *testing.T) {
		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(`{"content":"unknown"}`).
			Post("/api/shorten")
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "Expected status code 400 Bad Request")
	})

	// Test case: Batch request with gzip encoding
	t.Run("Batch request with gzip encoding", func(t *testing.T) {
		batchRequest := []map[string]string{
			{"correlation_id": "1", "original_url": "https://example1.com"},
			{"correlation_id": "2", "original_url": "https://example2.com"},
		}
		body, err := json.Marshal(batchRequest)
		require.NoError(t, err)

		var gzippedBody bytes.Buffer
		gz := gzip.NewWriter(&gzippedBody)
		_, err = gz.Write(body)
		require.NoError(t, err)
		require.NoError(t, gz.Close())

		resp, err := client.R().
			SetHeader("Content-Encoding", "gzip").
			SetHeader("Content-Type", "application/json").
			SetBody(gzippedBody.Bytes()).
			Post("/api/shorten/batch")
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode(), "Expected status code 201 Created")

		var results []map[string]string
		err = json.Unmarshal(resp.Body(), &results)
		require.NoError(t, err)

		require.Len(t, results, len(batchRequest), "Number of responses should match number of requests")
		for _, result := range results {
			assert.Contains(t, result["short_url"], cfg.BaseURL, "Short URL should start with base URL")
			assert.NotEmpty(t, result["correlation_id"], "Correlation ID should not be empty")
		}
	})
}
