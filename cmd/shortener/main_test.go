package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"github.com/go-resty/resty/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"

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
				endpoints.GetFullURL(w, r, storage)
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
	// 1) Spin up your server
	cfg := config.NewConfig()
	storeNotImported := store.NewMemoryStorage()
	router := endpoints.NewRouter(context.Background(), cfg, storeNotImported, "testversion")
	ts := httptest.NewServer(router)
	defer ts.Close()

	// =========== SUB-TEST #1: GZIPPED REQUEST, PLAIN RESPONSE =========== // I suspect that the server will
	t.Run("GzippedRequest_PlainResponse", func(t *testing.T) {

		httpClient := &http.Client{
			Transport: &http.Transport{
				DisableCompression: true,
			},
		}

		// 2) Create Resty client that does NOT auto-decompress
		//    so we can manually gzip.NewReader the response.
		client := resty.NewWithClient(httpClient).
			SetBaseURL(ts.URL).
			SetRedirectPolicy(resty.NoRedirectPolicy()).
			SetDoNotParseResponse(true) // If removed it will make GzippedRequest_PlainResponse pass.

		// We only send gzipped data in request; do NOT ask for gzip response.
		body := `{"url":"https://example.com"}`
		var gzippedBody bytes.Buffer
		gz := gzip.NewWriter(&gzippedBody)
		_, err := gz.Write([]byte(body))
		require.NoError(t, err, "Failed to write to gzip writer")
		require.NoError(t, gz.Close(), "Failed to close gzip writer")

		// Notice we do NOT set "Accept-Encoding: gzip".
		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Accept-Encoding", "").
			SetHeader("Content-Encoding", "gzip").
			SetBody(gzippedBody.Bytes()).
			Post("/api/shorten")
		require.NoError(t, err, "Request failed")

		dump, _ := httputil.DumpResponse(resp.RawResponse, false)
		t.Logf("[DEBUG] Raw HTTP response:\n%s", dump)

		rawBody, err := io.ReadAll(resp.RawResponse.Body)
		require.NoError(t, err, "Failed to read raw response body")

		require.False(t, isGzipData(rawBody), "Expected plain JSON")

		t.Logf("Raw body: %s", string(rawBody))

		var respData map[string]string
		err = json.Unmarshal(rawBody, &respData)
		require.NoError(t, err, "Failed to parse plain JSON response")
		require.Contains(t, respData["result"], cfg.BaseURL)

	})

	// =========== SUB-TEST #2: PLAIN REQUEST, GZIPPED RESPONSE ===========
	t.Run("PlainRequest_GzippedResponse", func(t *testing.T) {

		client := resty.New().
			SetBaseURL(ts.URL).
			SetRedirectPolicy(resty.NoRedirectPolicy())

		// We send normal JSON, but set "Accept-Encoding: gzip"
		body := `{"url":"https://example.com"}`
		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Accept-Encoding", "gzip"). // Asking for gzipped response
			SetDoNotParseResponse(true).
			SetBody([]byte(body)). // Plain JSON
			Post("/api/shorten")
		require.NoError(t, err, "Request failed")

		dump, _ := httputil.DumpResponse(resp.RawResponse, false) //  it will dump only the response headers (and the first line/status) but will not consume the body
		t.Logf("[DEBUG] Raw HTTP response:\n%s", dump)

		// The server should return 201 + gzipped JSON
		require.Equal(t, http.StatusCreated, resp.StatusCode())
		require.Equal(t, "gzip", resp.Header().Get("Content-Encoding"),
			"Expected gzipped response")

		// Manual read
		rawBytes, err := io.ReadAll(resp.RawResponse.Body)
		require.NoError(t, err, "Failed to read raw body")
		t.Logf("Raw response body length=%d, hex=%x", len(rawBytes), rawBytes)

		// Manual decompression
		gzr, err := gzip.NewReader(bytes.NewReader(rawBytes))
		require.NoError(t, err, "Failed to create gzip reader")
		decompressedBody, err := io.ReadAll(gzr)
		require.NoError(t, err, "Failed to decompress gzip response")

		var respData map[string]string
		err = json.Unmarshal(decompressedBody, &respData)
		require.NoError(t, err, "Failed to unmarshal response JSON")
		require.Contains(t, respData["result"], cfg.BaseURL)
	})

	// =========== SUB-TEST #3: BOTH GZIPPED REQUEST AND RESPONSE ===========
	t.Run("Both_GzippedRequestAndResponse", func(t *testing.T) {

		httpClient := &http.Client{
			Transport: &http.Transport{
				DisableCompression: true,
			},
		}

		// 2) Create Resty client that does NOT auto-decompress
		//    so we can manually gzip.NewReader the response.
		client := resty.NewWithClient(httpClient).
			SetBaseURL(ts.URL).
			SetRedirectPolicy(resty.NoRedirectPolicy()).
			SetDoNotParseResponse(true)

		t.Skip("Skipping this test while debugging others!")
		body := `{"url":"https://example.com"}`
		var gzippedBody bytes.Buffer
		gz := gzip.NewWriter(&gzippedBody)
		_, err := gz.Write([]byte(body))
		require.NoError(t, err)
		require.NoError(t, gz.Close())

		// Here we send gzipped data AND ask for gzipped response
		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Content-Encoding", "gzip"). // request is gzipped
			SetHeader("Accept-Encoding", "gzip").  // response is gzipped
			SetBody(gzippedBody.Bytes()).
			Post("/api/shorten")
		require.NoError(t, err, "Request failed")

		dump, _ := httputil.DumpResponse(resp.RawResponse, true)
		t.Logf("[DEBUG] Raw HTTP response:\n%s", dump)

		// We expect the server to return 201 + a gzip-encoded JSON
		require.Equal(t, http.StatusCreated, resp.StatusCode())
		require.Equal(t, "gzip", resp.Header().Get("Content-Encoding"),
			"Expected gzipped response")

		// Manually decompress the response
		gzr, err := gzip.NewReader(bytes.NewReader(resp.Body()))
		require.NoError(t, err, "Failed to create gzip reader")
		defer gzr.Close()

		decompressedBody, err := io.ReadAll(gzr)
		require.NoError(t, err, "Failed to decompress gzip response")
		t.Logf("Decompressed response body: %s", decompressedBody)

		var respData map[string]string
		err = json.Unmarshal(decompressedBody, &respData)
		require.NoError(t, err, "Failed to unmarshal response JSON")
		require.Contains(t, respData["result"], cfg.BaseURL)
	})
}

// isGzipData is just a convenience function to see if bytes begin with the gzip magic number.
func isGzipData(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1F && data[1] == 0x8B
}
