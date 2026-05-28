package middleware

import (
	"context"
	"net/http"
)

package middleware

import (
"context"
"net/http"

"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
"github.com/asbm77/go-musthave-diploma-tpl/pkg/logger"
)

// AuthMiddleware создаёт middleware для проверки аутентификации
// Возвращает middleware, который проверяет JWT токен и добавляет userID в контекст
func AuthMiddleware(jwtAuth *auth.JWTAuth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Проверяем токен
			userID, err := jwtAuth.ValidateToken(r)
			if err != nil {
				logger.Logger.Debugw("Authentication failed", "error", err, "path", r.URL.Path)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Добавляем userID в контекст запроса
			ctx := context.WithValue(r.Context(), auth.UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuthMiddleware создаёт middleware, который не требует обязательной аутентификации
// Если токен предоставлен и валиден, userID добавляется в контекст
// Если токен отсутствует или невалиден, запрос всё равно проходит дальше
func OptionalAuthMiddleware(jwtAuth *auth.JWTAuth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := jwtAuth.ValidateToken(r)
			if err == nil {
				ctx := context.WithValue(r.Context(), auth.UserIDKey, userID)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth является обёрткой для хендлеров, требующих аутентификации
// Используется для маршрутов, где нужна обязательная проверка
func RequireAuth(jwtAuth *auth.JWTAuth, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtAuth.ValidateToken(r)
		if err != nil {
			logger.Logger.Debugw("Authentication failed", "error", err, "path", r.URL.Path)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), auth.UserIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// GetUserIDFromContext получает userID из контекста запроса
// Это вспомогательная функция для использования в хендлерах
func GetUserIDFromContext(ctx context.Context) (int64, error) {
	return auth.GetUserIDFromContext(ctx)
}