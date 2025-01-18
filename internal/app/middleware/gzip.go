// Internal/app/middleware/gzip.go.

package middleware

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"
)

const gzipencoding = "gzip" // whyisthereaneedtodoit? its name is longer than the string linter was complaining over
const ContentEncodingHeader = "Content-Encoding"

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
	n, err := c.zw.Write(p)
	if err != nil {
		wrappedErr := errors.New("failed to write compressed data")
		Log.Error().Err(err).Msg("compressWriter: Write operation failed")
		return n, wrappedErr
	}
	return n, nil
}

func (c *compressWriter) WriteHeader(statusCode int) {
	c.w.Header().Set(ContentEncodingHeader, gzipencoding)
	c.w.WriteHeader(statusCode)
}

func (c *compressWriter) Close() error {
	if err := c.zw.Close(); err != nil {
		wrappedErr := errors.New("failed to close gzip writer")
		Log.Error().Err(err).Msg("compressWriter: Close operation failed")
		return wrappedErr
	}
	return nil
}

// compressReader wraps io.ReadCloser to transparently decompress request bodies.
type compressReader struct {
	r  io.ReadCloser
	zr *gzip.Reader
}

func newCompressReader(r io.ReadCloser) (*compressReader, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		wrappedErr := errors.New("failed to create gzip reader")
		Log.Error().Err(err).Msg("newCompressReader: Failed to initialize gzip reader")
		return nil, wrappedErr
	}
	return &compressReader{r: r, zr: zr}, nil
}

func (c *compressReader) Read(p []byte) (int, error) {
	n, err := c.zr.Read(p)
	if err != nil {
		wrappedErr := errors.New("failed to read from gzip reader")
		Log.Error().Err(err).Msg("compressReader: Read operation failed")
		return n, wrappedErr
	}
	return n, nil
}

func (c *compressReader) Close() error {
	if err := c.zr.Close(); err != nil {
		wrappedErr := errors.New("failed to close gzip reader")
		Log.Error().Err(err).Msg("compressReader: error closing gzip reader")
		return wrappedErr
	}

	if err := c.r.Close(); err != nil {
		wrappedErr := errors.New("failed to close underlying reader")
		Log.Error().Err(err).Msg("compressReader: error closing underlying reader")
		return wrappedErr
	}

	return nil
}

func GzipMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ow := w

		acceptEncoding := r.Header.Get("Accept-Encoding")
		supportsGzip := strings.Contains(acceptEncoding, "gzip")
		if supportsGzip {
			cw := newCompressWriter(w)
			ow = cw
			defer func(cw *compressWriter) {
				err := cw.Close()
				if err != nil {
					Log.Error().Err(err).Msg("compressWriter: error closing gzip writer")
				}
			}(cw)
		}

		contentEncoding := r.Header.Get("Content-Encoding")
		sendsGzip := strings.Contains(contentEncoding, "gzip")
		if sendsGzip {
			cr, err := newCompressReader(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			r.Body = cr
			defer func(cr *compressReader) {
				err := cr.Close()
				if err != nil {
					Log.Error().Err(err).Msg("compressReader: error closing gzip reader")
				}
			}(cr)
		}

		h.ServeHTTP(ow, r)
	})
}
