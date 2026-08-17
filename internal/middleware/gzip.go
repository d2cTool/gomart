package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// compressibleTypes — типы содержимого, которые имеет смысл сжимать.
var compressibleTypes = []string{"application/json", "text/plain", "text/html"}

// Gzip возвращает посредник, который распаковывает тело запроса, сжатое
// алгоритмом gzip, и сжимает ответ, если клиент указал поддержку gzip
// в заголовке Accept-Encoding.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(strings.ToLower(r.Header.Get("Content-Encoding")), "gzip") {
			reader, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "invalid gzip body", http.StatusBadRequest)
				return
			}
			defer func() { _ = reader.Close() }()
			r.Body = struct {
				io.Reader
				io.Closer
			}{Reader: reader, Closer: r.Body}
		}

		if !strings.Contains(strings.ToLower(r.Header.Get("Accept-Encoding")), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		cw := newCompressWriter(w)
		defer func() { _ = cw.Close() }()
		next.ServeHTTP(cw, r)
	})
}

// compressWriter сжимает тело ответа, если его тип относится к сжимаемым.
type compressWriter struct {
	w          http.ResponseWriter
	zw         *gzip.Writer
	compress   bool
	headerSent bool
}

// newCompressWriter оборачивает http.ResponseWriter сжимающей реализацией.
func newCompressWriter(w http.ResponseWriter) *compressWriter {
	return &compressWriter{w: w}
}

// Header возвращает заголовки ответа.
func (c *compressWriter) Header() http.Header {
	return c.w.Header()
}

// WriteHeader выставляет код ответа и решает, нужно ли сжимать тело.
func (c *compressWriter) WriteHeader(statusCode int) {
	if c.headerSent {
		return
	}
	c.headerSent = true

	if isCompressible(c.w.Header().Get("Content-Type")) {
		c.compress = true
		c.w.Header().Set("Content-Encoding", "gzip")
		c.w.Header().Del("Content-Length")
		c.zw = gzip.NewWriter(c.w)
	}
	c.w.WriteHeader(statusCode)
}

// Write пишет тело ответа, при необходимости сжимая его.
func (c *compressWriter) Write(p []byte) (int, error) {
	if !c.headerSent {
		c.WriteHeader(http.StatusOK)
	}
	if c.compress {
		return c.zw.Write(p)
	}
	return c.w.Write(p)
}

// Close завершает сжатие и сбрасывает буфер в исходный писатель.
func (c *compressWriter) Close() error {
	if c.zw == nil {
		return nil
	}
	return c.zw.Close()
}

// isCompressible сообщает, стоит ли сжимать содержимое указанного типа.
func isCompressible(contentType string) bool {
	contentType = strings.ToLower(contentType)
	for _, t := range compressibleTypes {
		if strings.Contains(contentType, t) {
			return true
		}
	}
	return false
}
