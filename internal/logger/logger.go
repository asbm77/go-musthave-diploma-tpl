// internal/logger/logger.go
package logger

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

var Logger *zap.SugaredLogger

// Initialize инициализирует глобальный логгер
func Initialize(level string) error {
	var zapLogger *zap.Logger
	var err error

	switch level {
	case "debug":
		zapLogger, err = zap.NewDevelopment()
	case "info", "production":
		zapLogger, err = zap.NewProduction()
	default:
		zapLogger, err = zap.NewProduction()
	}

	if err != nil {
		return err
	}

	Logger = zapLogger.Sugar()
	return nil
}

// HTTPLogger middleware для логирования HTTP запросов
func HTTPLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrappedWriter := &responseWriterWrapper{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrappedWriter, r)

		latency := time.Since(start)

		Logger.Infow("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrappedWriter.statusCode,
			"latency_ms", latency.Milliseconds(),
		)
	})
}

// responseWriterWrapper обёртка для http.ResponseWriter
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	bodySize   int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.bodySize += size
	return size, err
}
