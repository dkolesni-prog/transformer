// Internal/app/middleware/gzip.go.

package middleware

import (
	"compress/gzip"
	"errors"
	"io"
	"log"
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
	log.Printf("[compressWriter] wrote %d bytes to gzip (err=%v)\n", n, err)
	if err != nil {
		Log.Error().Err(err).Msg("Failed to write to gzip writer")
	}
	return n, err
}

func (c *compressWriter) WriteHeader(statusCode int) {

	c.w.Header().Del("Content-Length")
	if statusCode < 300 {
		log.Println("Check if entered WriteHeader")
		c.w.Header().Set("Content-Encoding", "gzip")
	}
	c.w.WriteHeader(statusCode)
}

func (c *compressWriter) Close() error {
	if err := c.zw.Close(); err != nil {
		Log.Error().Err(err).Msg("Failed to close gzip writer")
		log.Printf("[compressWriter] Close() returned: %v\n", err)
		return err
	}
	log.Printf("[compressWriter] Close() closed ok\n")
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
		Log.Error().Err(err).Msg("Failed to close gzip reader THISISTHEMARKTHATGZIPDATAOK")
		return err
	}
	if err := c.r.Close(); err != nil {
		log.Println("Failed to close underlying reader", err)
		Log.Error().Err(err).Msg("Failed to close underlying reader THISISTHEMARKTHATGZIPDATAOK")
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

		log.Println("accept-encoding", r.Header.Get("Accept-Encoding"))
		log.Println("content-encoding", r.Header.Get("Content-Encoding"))
		log.Println("now processing gzip middleware")

		ow := w

		log.Println("endpoint:", r.URL)
		log.Println("headers:", r.Header)
		acceptEncoding := r.Header.Get("Accept-Encoding")
		if strings.Contains(acceptEncoding, "gzip") {
			log.Println("CHANGING TO COMPRESS WRITER")
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
			log.Println("Status code was")
		}

		contentEncoding := r.Header.Get("Content-Encoding")
		if strings.Contains(contentEncoding, "gzip") {
			Log.Info().Msg("Request body is gzip-encoded; decompressing")
			log.Println("Before new")
			cr, err := newCompressReader(r.Body)
			log.Println("After new:", err)
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
			type statusCapturer interface {
				Status() int
			}

			if sc, ok := ow.(statusCapturer); ok {
				log.Printf("Status code was: %d", sc.Status())
			} else {
				log.Println("Status code is unknown (WriteHeader may not have been called yet)")
			}
		}
		log.Println("MARK")
		h.ServeHTTP(ow, r)
		log.Println("MARK2")
	})
}
