package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dkolesni-prog/transformer/internal/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	app.Initialize("info", "test-version")

	cfg := app.NewConfig()
	cfg.FileStoragePath = "test_storage.json" // Temporary test file
	defer os.Remove(cfg.FileStoragePath)      // Cleanup after tests

	storage := app.NewStorage(cfg.FileStoragePath)
	router := app.NewRouter(cfg, storage, "test-version")

	t.Run("POST /api/shorten with valid JSON", func(t *testing.T) {
		body := `{"url":"https://example.com"}`
		req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		var response map[string]string
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)
		result, ok := response["result"]
		require.True(t, ok)
		assert.True(t, strings.HasPrefix(result, cfg.BaseURL))
	})

	t.Run("POST /api/shorten with invalid JSON", func(t *testing.T) {
		body := `{"url": "https://example.com"` // Malformed JSON
		req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "Failed to parse JSON\n", rec.Body.String())
	})

	t.Run("Gzip compressed request", func(t *testing.T) {
		body := `{"url":"https://example.com"}`
		var gzippedBody bytes.Buffer
		gz := gzip.NewWriter(&gzippedBody)
		_, err := gz.Write([]byte(body))
		require.NoError(t, err)
		require.NoError(t, gz.Close())

		req := httptest.NewRequest(http.MethodPost, "/api/shorten", &gzippedBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "gzip")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		var response map[string]string
		err = json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(response["result"], cfg.BaseURL))
	})

	t.Run("Gzip compressed response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	})
}

func TestConfigurationPriority(t *testing.T) {
	// Backup original environment variables
	originalEnv := os.Getenv("FILE_STORAGE_PATH")
	defer os.Setenv("FILE_STORAGE_PATH", originalEnv)

	// Set environment variable
	os.Setenv("FILE_STORAGE_PATH", "env_test.json")

	cfg := app.NewConfig()
	assert.Equal(t, "env_test.json", cfg.FileStoragePath)

	// Test flag overriding environment variable
	os.Unsetenv("FILE_STORAGE_PATH") // Remove env for testing flags
	os.Args = []string{"cmd", "-f", "flag_test.json"}
	cfg = app.NewConfig()
	assert.Equal(t, "flag_test.json", cfg.FileStoragePath)
}
