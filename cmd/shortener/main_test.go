// Cmd/shortener/main_test.go.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dkolesni-prog/transformer/internal/app"
	"github.com/stretchr/testify/assert"
)

// TestEndpoints tests the main endpoints of the URL shortener service.
func TestEndpoints(t *testing.T) {
	// Initialize the logger for tests (writing to os.Stdout for visibility)
	app.Initialize("info", "test-version")

	// Initialize configuration and storage
	cfg := app.NewConfig()
	storage := app.NewStorage()

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
			// -- CHANGED: now we send raw URL in the body instead of form-encoded
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
			// -- CHANGED: code returns 400 and "Failed to parse JSON\n"
			//             if JSON is malformed
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
			// -- CHANGED: code returns 400 and "Empty url field in JSON\n"
			//             if "url" field is missing
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
			// -- CHANGED: code returns 400 and "Invalid URL\n"
			//             if URL is invalid
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
				// /api/shorten returns JSON {"result": "..."}
				switch {
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
					// We only check that the body starts with BaseURL
					assert.True(t, strings.HasPrefix(rec.Body.String(), tt.wantBody),
						"Response body should start with BaseURL")

				default:
					// For other cases or error responses, compare exact body
					assert.Equal(t, tt.wantBody, rec.Body.String(),
						"Response body should match expected")
				}
			}

			// Check the headers
			for key, wantValue := range tt.wantHeader {
				gotValue := rec.Header().Get(key)
				assert.Equal(t, wantValue, gotValue, "Header %s should match", key)
			}
		})
	}
}
