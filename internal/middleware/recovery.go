package middleware

import (
	"net/http"

	"github.com/asbm77/go-musthave-diploma-tpl/pkg/logger"
)

// RecoveryMiddleware восстанавливает после паник и возвращает 500 ошибку
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Logger.Errorw("Panic recovered",
					"error", err,
					"method", r.Method,
					"path", r.URL.Path,
				)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
