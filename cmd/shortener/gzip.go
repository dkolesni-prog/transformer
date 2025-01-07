//Cmd/shortener/gzip.go

package main

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"

	"github.com/dkolesni-prog/transformer/internal/app"
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

func (c *compressWriter) Write(p []byte) (int, error) {
	n, err := c.zw.Write(p)
	if err != nil {
		// 1) Log the original error with context:
		app.Log.Error().Err(err).Msg("gzip writer write error")

		// 2) Return a new error that includes the original error text.
		return n, errors.New("gzip writer write: " + err.Error())
	}
	return n, nil
}

func (c *compressWriter) Header() http.Header {
	return c.w.Header()
}

func (c *compressWriter) WriteHeader(statusCode int) {
	if statusCode < 300 {
		c.w.Header().Set("Content-Encoding", "gzip")
	}
	c.w.WriteHeader(statusCode)
}

func (c *compressWriter) Close() error {
	if err := c.zw.Close(); err != nil {
		app.Log.Error().Err(err).Msg("gzip writer close error")
		return errors.New("gzip writer close: " + err.Error())
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
		app.Log.Error().Err(err).Msg("create gzip reader error")
		return nil, errors.New("create gzip reader: " + err.Error())
	}
	return &compressReader{r: r, zr: zr}, nil
}

func (c *compressReader) Read(p []byte) (int, error) {
	n, err := c.zr.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		// log + wrap the error
		app.Log.Error().Err(err).Msg("gzip reader read error")
		return n, errors.New("gzip reader read: " + err.Error())
	}
	return n, nil
}

func (c *compressReader) Close() error {
	if err := c.r.Close(); err != nil {
		app.Log.Error().Err(err).Msg("closing underlying reader")
		return errors.New("closing underlying reader: " + err.Error())
	}
	if err := c.zr.Close(); err != nil {
		app.Log.Error().Err(err).Msg("gzip reader close error")
		return errors.New("gzip reader close: " + err.Error())
	}
	return nil
}
