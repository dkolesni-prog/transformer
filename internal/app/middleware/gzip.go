// Internal/app/middleware/gzip.go.
package middleware

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

const (
	contentEncodingHeader = "Content-Encoding"
	acceptEncodingHeader  = "Accept-Encoding"
	gzipEncoding          = "gzip"
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
	return &compressWriter{w: w, zw: zw}
}

func (c *compressWriter) Header() http.Header {
	return c.w.Header()
}

func (c *compressWriter) Write(p []byte) (int, error) {
	n, err := c.zw.Write(p)
	if err != nil {
		Log.Error().Err(err).Msg("Failed to write to gzip writer")
		return n, fmt.Errorf("compressWriter write: %w", err)
	}
	log.Printf("[compressWriter] wrote %d bytes to gzip (err=%v)\n", n, err)
	return n, nil
}

func (c *compressWriter) WriteHeader(statusCode int) {
	c.w.Header().Del("Content-Length")
	if statusCode < http.StatusMultipleChoices {
		log.Println("Check if entered WriteHeader")
		c.w.Header().Set(contentEncodingHeader, gzipEncoding)
	}
	c.w.WriteHeader(statusCode)
}

func (c *compressWriter) Close() error {
	if err := c.zw.Close(); err != nil {
		Log.Error().Err(err).Msg("Failed to close gzip writer")
		log.Printf("[compressWriter] Close() returned: %v\n", err)
		return fmt.Errorf("closing gzip writer: %w", err)
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
		return nil, fmt.Errorf("creating gzip reader: %w", err)
	}
	return &compressReader{r: r, zr: zr}, nil
}

func (c *compressReader) Read(p []byte) (int, error) {
	n, err := c.zr.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		Log.Error().Err(err).Msg("Failed to read from gzip reader")
		return n, fmt.Errorf("reading gzip data: %w", err)
	}
	return n, err
}

func (c *compressReader) Close() error {
	if err := c.zr.Close(); err != nil {
		Log.Error().Err(err).Msg("Failed to close gzip reader")
		return fmt.Errorf("closing gzip reader: %w", err)
	}
	if err := c.r.Close(); err != nil {
		log.Println("Failed to close underlying reader", err)
		Log.Error().Err(err).Msg("Failed to close underlying reader")
		return fmt.Errorf("closing underlying reader: %w", err)
	}
	return nil
}

// GzipMiddleware handles both gzip compression (response) and decompression (request).
func GzipMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Log.Info().
			Str(acceptEncodingHeader, r.Header.Get(acceptEncodingHeader)).
			Str(contentEncodingHeader, r.Header.Get(contentEncodingHeader)).
			Msg("Processing gzip middleware")

		log.Println("accept-encoding", r.Header.Get(acceptEncodingHeader))
		log.Println("content-encoding", r.Header.Get(contentEncodingHeader))
		log.Println("endpoint:", r.URL)
		log.Println("headers:", r.Header)

		ow := w
		if strings.Contains(r.Header.Get(acceptEncodingHeader), gzipEncoding) {
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
		}

		if strings.Contains(r.Header.Get(contentEncodingHeader), gzipEncoding) {
			Log.Info().Msg("Request body is gzip-encoded; decompressing")
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
		}
		h.ServeHTTP(ow, r)
	})
}
