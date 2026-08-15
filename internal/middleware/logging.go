package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Logging возвращает посредник, который пишет в лог сведения о каждом
// обработанном HTTP-запросе: метод, путь, код ответа, размер тела и
// длительность обработки.
func Logging(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rw, r)

			log.Info("request handled",
				zap.String("method", r.Method),
				zap.String("uri", r.RequestURI),
				zap.Int("status", rw.status),
				zap.Int("size", rw.size),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}

// responseRecorder запоминает код ответа и размер отданного тела.
type responseRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

// WriteHeader запоминает код ответа и передаёт его дальше.
func (r *responseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

// Write запоминает размер отданного тела и передаёт его дальше.
func (r *responseRecorder) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.size += n
	return n, err
}
