// Cmd/shortener/main_test.go.
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dkolesni-prog/transformer/internal/app"
	"github.com/go-chi/chi/v5"
)

// TestEndpoints тестирует основные эндпоинты сервиса коротких URL.
func TestEndpoints(t *testing.T) {
	// Инициализация конфигурации и хранилища
	cfg := app.NewConfig()
	storage := app.NewStorage()

	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		setup      func(*app.Storage) // Функция для предварительной настройки хранилища
		wantCode   int
		wantBody   string
		wantHeader map[string]string
	}{
		{
			name:     "POST valid URL",
			method:   http.MethodPost,
			url:      "/",
			body:     "https://example.com",
			setup:    func(s *app.Storage) {},
			wantCode: http.StatusCreated,
			// Проверяем, что тело ответа начинается с BaseURL
			wantBody: cfg.BaseURL,
			wantHeader: map[string]string{
				"Content-Type": "text/plain",
			},
		},
		{
			name:   "GET valid short URL",
			method: http.MethodGet,
			url:    "/abcd1234",
			body:   "",
			setup: func(s *app.Storage) {
				s.Set("abcd1234", "https://example.com")
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Настройка хранилища перед каждым тестом
			if tt.setup != nil {
				tt.setup(storage)
			}

			// Создание запроса и рекордера
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.url, strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, tt.url, http.NoBody)
			}
			rec := httptest.NewRecorder()

			// Настройка маршрутизатора chi
			r := chi.NewRouter()

			// Регистрация обработчиков
			r.Post("/", func(w http.ResponseWriter, r *http.Request) {
				app.ShortenURL(w, r, storage, cfg.BaseURL)
			})

			r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
				app.GetFullURL(w, r, storage)
			})

			// Обработка запроса
			r.ServeHTTP(rec, req)

			// Проверка кода статуса
			if rec.Code != tt.wantCode {
				t.Errorf("получен код статуса %d, ожидается %d", rec.Code, tt.wantCode)
			}

			// Проверка тела ответа
			if tt.wantBody != "" && !strings.HasPrefix(rec.Body.String(), tt.wantBody) {
				t.Errorf("получено тело %q, ожидается префикс %q", rec.Body.String(), tt.wantBody)
			}

			// Проверка заголовков
			for key, wantValue := range tt.wantHeader {
				gotValue := rec.Header().Get(key)
				if gotValue != wantValue {
					t.Errorf("получен заголовок %q=%q, ожидается %q=%q", key, gotValue, key, wantValue)
				}
			}
		})
	}
}
