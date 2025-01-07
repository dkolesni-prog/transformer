package app

import (
	"compress/gzip"
	"io"
	"net/http"
)

type compressWriter struct {
	w  http.ResponseWriter
	zw *gzip.Writer
}

func newCompressWriter(w http.ResponseWriter) *compressWriter {
	return &compressWriter{
		w:  w,
		zw: gzip.NewWriter(w),
	}
}

func (c *compressWriter) Header() http.Header {
	return c.w.Header()
}

func (c *compressWriter) Write(p []byte) (int, error) {
	return c.zw.Write(p)
}

func (c *compressWriter) WriteHeader(statusCode int) {
	c.w.Header().Set("Content-Encoding", "gzip")
	c.w.WriteHeader(statusCode)
}

func (c *compressWriter) Close() error {
	return c.zw.Close()
}

// compressReader wraps io.ReadCloser to transparently decompress request bodies.
type compressReader struct {
	r  io.ReadCloser
	zr *gzip.Reader
}

func newCompressReader(r io.ReadCloser) (*compressReader, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	return &compressReader{r: r, zr: zr}, nil
}

func (c *compressReader) Read(p []byte) (int, error) {
	return c.zr.Read(p)
}

func (c *compressReader) Close() error {
	if err := c.zr.Close(); err != nil {
		return err
	}
	return c.r.Close()
}

// GzipMiddleware handles gzip compression/decompression for requests and responses.
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decompress gzipped request body
		if r.Header.Get("Content-Encoding") == "gzip" {
			cr, err := newCompressReader(r.Body)
			if err != nil {
				http.Error(w, "Failed to decode gzip", http.StatusBadRequest)
				return
			}
			defer cr.Close()

			r.Body = cr
			r.Header.Del("Content-Encoding")
		}

		// Compress responses if client supports gzip
		if r.Header.Get("Accept-Encoding") == "gzip" {
			cw := newCompressWriter(w)
			defer cw.Close()
			w = cw
			w.Header().Set("Content-Encoding", "gzip")
		}

		next.ServeHTTP(w, r)
	})
}
