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

type CompressedReader interface {
	Read(p []byte) (int, error)
	Close() error
}

type compressWriter struct {
	w  http.ResponseWriter
	zw *gzip.Writer
}

func newCompressWriter(w http.ResponseWriter) *compressWriter {
	zw, _ := gzip.NewWriterLevel(w, gzip.BestCompression)
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
		return n, errors.New("failed to write to gzip writer")
	}
	return n, nil
}

func (c *compressWriter) WriteHeader(statusCode int) {
	if statusCode < 300 {
		c.w.Header().Set(ContentEncodingHeader, gzipencoding)
	}
	c.w.WriteHeader(statusCode)
}

func (c *compressWriter) Close() error {
	if err := c.zw.Close(); err != nil {
		Log.Error().Err(err).Msg("Failed to close gzip writer")
		return errors.New("failed to close gzip writer")
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
		return nil, errors.New("failed to create gzip reader")
	}
	return &compressReader{zr: zr, r: r}, nil
}

func (c *compressReader) Read(p []byte) (n int, err error) {
	n, err = c.zr.Read(p)
	if err != nil {
		Log.Error().Err(err).Msg("Failed to read from compressed reader")
		return n, errors.New("failed to read from compressed reader")
	}
	return n, nil
}

func (c *compressReader) Close() error {
	if err := c.zr.Close(); err != nil {
		Log.Error().Err(err).Msg("Failed to close compressed reader")
		return errors.New("failed to close compressed reader")
	}
	if err := c.r.Close(); err != nil {
		Log.Error().Err(err).Msg("Failed to close underlying reader")
		return errors.New("failed to close underlying reader")
	}
	return nil
}

func GzipMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ow := w

		acceptEncoding := r.Header.Get("Accept-Encoding")
		supportsGzip := strings.Contains(acceptEncoding, gzipencoding)
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

		contentEncoding := r.Header.Get(ContentEncodingHeader)
		sendsGzip := strings.Contains(contentEncoding, gzipencoding)
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
