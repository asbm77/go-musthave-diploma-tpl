package middleware

import (
	"context"
	"net/http"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/pkg/logger"
)

// RequireAuth создаёт middleware для обязательной аутентификации
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
