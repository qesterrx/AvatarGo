package middleware

import (
	"net/http"
	"time"

	"github.com/qesterrx/AvatarGo/internal/logger"
)

// Middleware
type logResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (lrw *logResponseWriter) Write(msg []byte) (int, error) {
	size, err := lrw.ResponseWriter.Write(msg)
	lrw.size = size
	return size, err
}

func (lrw *logResponseWriter) WriteHeader(statusCode int) {
	lrw.ResponseWriter.WriteHeader(statusCode)
	lrw.statusCode = statusCode
}

// LoggingMiddleware - Middleware-фунция логирования http запросов
// Уровень логирования задан Info
// Отображает:
// URI - какой адрес вызывается
// method - метод обращения
// duration - длительность выполнения
// code - код результата оброботки
// size request - размер запроса (в байтах)
// size response - размер ответа (в байтах)
func LoggingMiddleware(h http.Handler) http.Handler {
	loggedHandler := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		lrw := logResponseWriter{ResponseWriter: w}

		h.ServeHTTP(&lrw, r)

		duration := time.Since(start)

		logger.Log.Info("URI: %s, method: %s, duration: %s, code: %d, size req: %d, size res: %d", r.RequestURI, r.Method, duration.String(), lrw.statusCode, r.ContentLength, lrw.size)

	}

	return http.HandlerFunc(loggedHandler)
}
