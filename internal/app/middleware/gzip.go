// Internal/app/middleware/gzip.go.

package middleware

import (
	"compress/gzip"
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
	return c.zw.Write(p)
}

func (c *compressWriter) WriteHeader(statusCode int) {
	if statusCode < 300 {
		c.w.Header().Set(ContentEncodingHeader, gzipencoding)
	}
	c.w.WriteHeader(statusCode)
}

func (c *compressWriter) Close() error {
	return c.zw.Close()
}

type compressReader struct {
	r  io.ReadCloser
	zr *gzip.Reader
}

func newCompressReader(r io.ReadCloser) (*compressReader, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}

	return &compressReader{
		r:  r,
		zr: zr,
	}, nil
}

func (c compressReader) Read(p []byte) (n int, err error) {
	return c.zr.Read(p)
}

func (c *compressReader) Close() error {
	if err := c.r.Close(); err != nil {
		return err
	}
	return c.zr.Close()
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
