// Internal/app/middleware/gzip.go.

package middleware

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
)

type compressWriter struct {
	w  http.ResponseWriter
	zw *gzip.Writer
}

func newCompressWriter(w http.ResponseWriter) *compressWriter {
	zw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		Log.Error().Err(err).Msg("Failed to create gzip writer")
		return nil
	}
	return &compressWriter{
		w:  w,
		zw: zw,
	}
}

func (c *compressWriter) Header() http.Header {
	return c.w.Header()
}

func (c *compressWriter) Write(p []byte) (int, error) {
	n, err := c.zw.Write(p)
	if err != nil {
		Log.Error().Err(err).Msg("Failed to write to gzip writer")
	}
	return n, err
}

func (c *compressWriter) WriteHeader(statusCode int) {
	if statusCode < 300 {
		c.w.Header().Set("Content-Encoding", "gzip")
	}
	c.w.WriteHeader(statusCode)
}

func (c *compressWriter) Close() error {
	if err := c.zw.Close(); err != nil {
		Log.Error().Err(err).Msg("Failed to close gzip writer")
		return err
	}
	return nil
}

type compressReader struct {
	r  io.ReadCloser
	zr *gzip.Reader
}

func newCompressReader(r io.ReadCloser) (*compressReader, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		Log.Error().Err(err).Msg("Failed to create gzip reader")
		return nil, err
	}
	return &compressReader{
		r:  r,
		zr: zr,
	}, nil
}

func (c *compressReader) Read(p []byte) (int, error) {
	n, err := c.zr.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		Log.Error().Err(err).Msg("Failed to read from gzip reader")
	}
	return n, err
}

func (c *compressReader) Close() error {
	if err := c.zr.Close(); err != nil {
		Log.Error().Err(err).Msg("Failed to close gzip reader")
		return err
	}
	if err := c.r.Close(); err != nil {
		Log.Error().Err(err).Msg("Failed to close underlying reader")
		return err
	}
	return nil
}

func GzipMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Log.Info().
			Str("Accept-Encoding", r.Header.Get("Accept-Encoding")).
			Str("Content-Encoding", r.Header.Get("Content-Encoding")).
			Msg("Processing gzip middleware")

		ow := w

		acceptEncoding := r.Header.Get("Accept-Encoding")
		if strings.Contains(acceptEncoding, "gzip") {
			Log.Info().Msg("Client supports gzip encoding; wrapping response writer")
			cw := newCompressWriter(w)
			if cw == nil {
				http.Error(w, "Failed to create gzip writer", http.StatusInternalServerError)
				return
			}
			ow = cw
			defer func() {
				if err := cw.Close(); err != nil {
					Log.Error().Err(err).Msg("Error closing compressWriter")
				}
			}()
		}

		contentEncoding := r.Header.Get("Content-Encoding")
		if strings.Contains(contentEncoding, "gzip") {
			Log.Info().Msg("Request body is gzip-encoded; decompressing")
			cr, err := newCompressReader(r.Body)
			if err != nil {
				Log.Error().Err(err).Msg("Failed to create gzip reader for request")
				http.Error(w, "Invalid gzip stream", http.StatusBadRequest)
				return
			}
			r.Body = cr
			defer func() {
				if err := cr.Close(); err != nil {
					Log.Error().Err(err).Msg("Error closing compressReader")
				}
			}()
		}

		h.ServeHTTP(ow, r)
	})
}
