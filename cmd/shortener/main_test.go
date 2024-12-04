// test_endpoints_test.go
package main

import (
	"github.com/dkolesni-prog/transformer/internal/app"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEndpoints(t *testing.T) {
	// Используем config.NewConfig() для получения конфигурации
	cfg := NewConfig()

	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		setup      func(map[string]string, map[string]string) // Функция для предварительной настройки карт
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
			// Обновляем wantBody, чтобы использовать cfg.BaseURL
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
			name:   "GET nonexistent short URL",
			method: http.MethodGet,
			url:    "/nonexistent",
			body:   "",
			setup:  func(keyLongValueShort, keyShortValueLong map[string]string) {},
			// Обновляем wantCode на http.StatusNotFound
			wantCode: http.StatusNotFound,
			wantBody: "Short URL not found\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Инициализируем карты
			keyLongValueShort := map[string]string{}
			keyShortValueLong := map[string]string{}

			// Настраиваем тестовый случай
			if tt.setup != nil {
				tt.setup(keyLongValueShort, keyShortValueLong)
			}

			// Создаем запрос и рекордер
			req := httptest.NewRequest(tt.method, tt.url, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			// Используем chi.Router вместо http.ServeMux для соответствия основному коду
			// Создаем роутер chi
			r := chi.NewRouter()

			// Определяем маршруты
			r.Post("/", func(w http.ResponseWriter, r *http.Request) {
				app.ShortenURL(w, r, keyShortValueLong, cfg.BaseURL)
			})

			r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
				app.GetFullURL(w, r, keyShortValueLong)
			})

			// Обрабатываем запрос
			r.ServeHTTP(rec, req)

			// Проверки
			if tt.wantCode != 0 {
				if rec.Code != tt.wantCode {
					t.Errorf("получен код статуса %d, ожидается %d", rec.Code, tt.wantCode)
				}
			}

			if tt.wantBody != "" {
				if !strings.HasPrefix(rec.Body.String(), tt.wantBody) {
					t.Errorf("получено тело %q, ожидается префикс %q", rec.Body.String(), tt.wantBody)
				}
			}

			for key, wantValue := range tt.wantHeader {
				gotValue := rec.Header().Get(key)
				if gotValue != wantValue {
					t.Errorf("получен заголовок %q=%q, ожидается %q=%q", key, gotValue, key, wantValue)
				}
			}
		})
	}
	return
}
