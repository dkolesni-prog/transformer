// Cmd/shortener/main_test.go.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dkolesni-prog/transformer/internal/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEndpoints tests the main endpoints of the URL shortener service.
func TestEndpoints(t *testing.T) {
	// Initialize the logger for tests (writing to os.Stdout for visibility)
	app.Initialize("info", "test-version")

	// Initialize configuration and storage
	cfg := app.NewConfig()
	storage := app.NewStorage(cfg.FileStoragePath)

	tests := []struct {
		name        string
		method      string
		url         string
		body        string
		contentType string
		setup       func(*app.Storage) // Function to preconfigure storage
		wantCode    int
		wantBody    string
		wantHeader  map[string]string
	}{
		{
			// now we send raw URL in the body instead of form-encoded
			name:        "POST valid URL (raw body)",
			method:      http.MethodPost,
			url:         "/",
			body:        "https://example.com",
			contentType: "text/plain",
			setup:       func(s *app.Storage) {},
			wantCode:    http.StatusCreated,
			// Check that the response body starts with BaseURL
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
			setup: func(s *app.Storage) {
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
			setup:    func(s *app.Storage) {},
			wantCode: http.StatusNotFound,
			wantBody: "Short URL not found\n",
		},
		// New Test Cases for /api/shorten
		{
			name:        "POST valid URL (JSON)",
			method:      http.MethodPost,
			url:         "/api/shorten",
			body:        `{"url":"https://example.com"}`,
			contentType: "application/json",
			setup:       func(s *app.Storage) {},
			wantCode:    http.StatusCreated,
			// The response JSON should have "result" which starts with BaseURL
			wantBody: cfg.BaseURL,
			wantHeader: map[string]string{
				"Content-Type": "application/json; charset=utf-8",
			},
		},
		{
			// code returns 400 and "Failed to parse JSON\n" if JSON is malformed
			name:        "POST invalid JSON",
			method:      http.MethodPost,
			url:         "/api/shorten",
			body:        `{"url": "https://example.com`, // Malformed JSON
			contentType: "application/json",
			setup:       func(s *app.Storage) {},
			wantCode:    http.StatusBadRequest,
			wantBody:    "Failed to parse JSON\n",
		},
		{
			// code returns 400 and "Empty url field in JSON\n" if "url" is missing
			name:        "POST missing URL field",
			method:      http.MethodPost,
			url:         "/api/shorten",
			body:        `{}`,
			contentType: "application/json",
			setup:       func(s *app.Storage) {},
			wantCode:    http.StatusBadRequest,
			wantBody:    "Empty url field in JSON\n",
		},
		{
			// code returns 400 and "Invalid URL\n" if URL is invalid
			name:        "POST invalid URL format",
			method:      http.MethodPost,
			url:         "/api/shorten",
			body:        `{"url":"invalid-url"}`,
			contentType: "application/json",
			setup:       func(s *app.Storage) {},
			wantCode:    http.StatusBadRequest,
			wantBody:    "Invalid URL\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup storage before each test
			if tt.setup != nil {
				tt.setup(storage)
			}

			// Create the request and recorder
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.url, strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, tt.url, http.NoBody)
			}

			// Set Content-Type if necessary
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			rec := httptest.NewRecorder()

			// Setup the router using app.NewRouter
			router := app.NewRouter(cfg, storage, "test-version")

			// Serve the request
			router.ServeHTTP(rec, req)

			// Check the status code
			assert.Equal(t, tt.wantCode, rec.Code, "Expected status code to match")

			// Check the response body
			if tt.wantBody != "" {
				switch {
				// /api/shorten returns JSON {"result": "..."}
				case tt.url == "/api/shorten" && strings.HasPrefix(tt.wantBody, cfg.BaseURL) && rec.Code == http.StatusCreated:
					// JSON response
					var responseData map[string]string
					err := json.Unmarshal(rec.Body.Bytes(), &responseData)
					assert.NoError(t, err, "Error unmarshaling JSON response")
					result, exists := responseData["result"]
					assert.True(t, exists, "Response should contain 'result' key")
					assert.True(t, strings.HasPrefix(result, cfg.BaseURL),
						"Result URL should start with BaseURL")

				case tt.url == "/" && rec.Code == http.StatusCreated:
					// Plain text response from ShortenURL
					// Check that body starts with BaseURL
					assert.True(t, strings.HasPrefix(rec.Body.String(), tt.wantBody),
						"Response body should start with BaseURL")

				default:
					// For other cases or error responses, compare exact body
					assert.Equal(t, tt.wantBody, rec.Body.String(),
						"Response body should match expected")
				}
			}

			// Check headers
			for key, wantValue := range tt.wantHeader {
				gotValue := rec.Header().Get(key)
				assert.Equal(t, wantValue, gotValue, "Header %s should match", key)
			}
		})
	}
}

// TestGzipCompression demonstrates how to test that your service
// can both accept gzip-compressed requests and return gzipped responses.
func TestGzipCompression(t *testing.T) {
	// 1. Initialize logger & config
	app.Initialize("info", "test-version")
	cfg := app.NewConfig()
	storage := app.NewStorage(cfg.FileStoragePath)

	// 2. Build the router + GZIP middleware
	router := app.NewRouter(cfg, storage, "test-version")
	// If your gzipMiddleware is defined as func gzipMiddleware(next http.Handler) http.Handler,
	// wrap the router:
	wrappedHandler := gzipMiddleware(router)

	// 3. Spin up an httptest server to test real HTTP requests
	srv := httptest.NewServer(wrappedHandler)
	defer srv.Close()

	// 4. Body we will send in POST requests
	requestBody := "https://example.com"

	// We'll assume the server responds with a short link that starts with cfg.BaseURL
	t.Run("sends_gzip", func(t *testing.T) {
		// Compress the request body
		gzippedBuf := bytes.NewBuffer(nil)
		zipWriter := gzip.NewWriter(gzippedBuf)
		_, err := zipWriter.Write([]byte(requestBody))
		require.NoError(t, err, "error writing to gzip writer")
		require.NoError(t, zipWriter.Close(), "error closing gzip writer")

		// Build the request
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/", gzippedBuf)
		require.NoError(t, err)
		// Indicate we send gzipped data
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Content-Type", "text/plain")

		// Do the request
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Disallow following redirects
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() {
			if cerr := resp.Body.Close(); cerr != nil {
				app.Log.Error().Err(cerr).Msg("failed to close response body")
			}
		}()

		// We expect HTTP 201 Created from the shortener's POST endpoint
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		// Read response body
		bodyBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		respBody := string(bodyBytes)

		// Check that the response starts with cfg.BaseURL
		require.True(t,
			strings.HasPrefix(respBody, cfg.BaseURL),
			"Expected response body %q to start with %q", respBody, cfg.BaseURL)
	})

	t.Run("accepts_gzip", func(t *testing.T) {
		// Put something in the storage so GET /somecode -> redirect
		storage.SetIfAbsent("abcd1234", "https://example.com")

		// Build a GET request
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/abcd1234", http.NoBody)
		require.NoError(t, err)
		// Indicate we want GZIP in the response
		req.Header.Set("Accept-Encoding", "gzip")

		// Do the request
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Disallow following redirects
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() {
			if cerr := resp.Body.Close(); cerr != nil {
				// log the error
				app.Log.Error().Err(cerr).Msg("failed to close response body")
			}
		}()

		// For your shortener, GET /abcd1234 should redirect to https://example.com
		require.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
		location := resp.Header.Get("Location")
		require.Equal(t, "https://example.com", location, "Location header mismatch")

		// Typically a redirect won't contain a huge body to compress,
		// but if you had a 200 OK + JSON body scenario, you'd read & decompress it:
		// gzReader, err := gzip.NewReader(resp.Body)
		// require.NoError(t, err)
		// decompressed, err := io.ReadAll(gzReader)
		// require.NoError(t, err)
		// require.JSONEq(t, someExpectedJSON, string(decompressed))
	})
}
