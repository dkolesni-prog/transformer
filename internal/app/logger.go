//// internal/app/logger.go
//

package app

import (
	"bytes"
	"github.com/rs/zerolog"
	"io"
	"net/http"
	"os"
	"time"
)

var Log = zerolog.Nop()

func Initialize(level string, version string) {
	parsedLevel, _ := zerolog.ParseLevel(level)
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().
		Str("version", version).
		Timestamp().
		Logger().Level(parsedLevel)
	Log = logger
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
	buffer     bytes.Buffer
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	rw.buffer.Write(b)
	return size, err
}

func WithLogging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var requestBody bytes.Buffer
		if r.Body != nil {
			tee := io.TeeReader(r.Body, &requestBody)
			r.Body = io.NopCloser(tee)
		}

		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		//обработка запроса
		h.ServeHTTP(ww, r)

		duration := time.Since(start)

		Log.Info().
			Str("uri", r.RequestURI).
			Str("method", r.Method).
			Dur("duration", duration).
			Str("request_body", requestBody.String()).
			Msg("Запрос получен")

		Log.Info().
			Int("status", ww.statusCode).
			Int("size", ww.size).
			Str("response_body", ww.buffer.String()).
			Msg("Ответ отправлен")
	})
}
