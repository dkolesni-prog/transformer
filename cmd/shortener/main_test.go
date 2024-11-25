package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		setup      func(map[string]string, map[string]string) // Function to prepopulate maps
		wantCode   int
		wantBody   string
		wantHeader map[string]string
	}{
		{
			name:     "POST valid URL",
			method:   http.MethodPost,
			url:      "/",
			body:     "https://example.com",
			setup:    func(keyLongValueShort, keyShortValueLong map[string]string) {},
			wantCode: http.StatusCreated,
			wantBody: "http://localhost:8080/",
			wantHeader: map[string]string{
				"Content-Type": "text/plain",
			},
		},
		{
			name:   "GET valid short URL",
			method: http.MethodGet,
			url:    "/abcd1234",
			body:   "",
			setup: func(keyLongValueShort, keyShortValueLong map[string]string) {
				keyShortValueLong["abcd1234"] = "https://example.com"
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
			setup:    func(keyLongValueShort, keyShortValueLong map[string]string) {},
			wantCode: http.StatusBadRequest,
			wantBody: "Short URL not found\n",
		},
		{
			name:     "Invalid method",
			method:   http.MethodPut,
			url:      "/",
			body:     "",
			setup:    func(keyLongValueShort, keyShortValueLong map[string]string) {},
			wantCode: http.StatusBadRequest,
			wantBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize maps
			keyLongValueShort := map[string]string{}
			keyShortValueLong := map[string]string{}

			// Setup test case
			if tt.setup != nil {
				tt.setup(keyLongValueShort, keyShortValueLong)
			}

			// Create request and recorder
			req := httptest.NewRequest(tt.method, tt.url, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			// Create handler
			handler := http.NewServeMux()
			handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/" {
					firstEndpoint(w, r, keyLongValueShort, keyShortValueLong)
				} else if r.Method == http.MethodGet && len(r.URL.Path) > 1 {
					secondEndpoint(w, r, keyShortValueLong)
				} else {
					w.WriteHeader(http.StatusBadRequest)
				}
			})

			// Serve the request
			handler.ServeHTTP(rec, req)

			// Assertions
			if tt.wantCode != 0 {
				if rec.Code != tt.wantCode {
					t.Errorf("got status code %d, want %d", rec.Code, tt.wantCode)
				}
			}

			if tt.wantBody != "" {
				if !strings.HasPrefix(rec.Body.String(), tt.wantBody) {
					t.Errorf("got body %q, want prefix %q", rec.Body.String(), tt.wantBody)
				}
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
