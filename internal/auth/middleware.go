// internal/auth/middleware.go
package auth

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const (
	UserLoginKey contextKey = "user_login"
	UserIDKey    contextKey = "user_id"
)

// JWTAuth управляет JWT аутентификацией
type JWTAuth struct {
	secret []byte
}

// NewJWTAuth создаёт новый экземпляр JWTAuth
func NewJWTAuth(secret string) *JWTAuth {
	return &JWTAuth{
		secret: []byte(secret),
	}
}

// GenerateToken создаёт новый JWT токен для пользователя
func (a *JWTAuth) GenerateToken(login string) (string, error) {
	token := NewToken(login, a.secret)
	return token.Generate()
}

// ValidateToken проверяет валидность JWT токена и возвращает логин пользователя
func (a *JWTAuth) ValidateToken(tokenString string) (string, error) {
	token := &Token{
		Secret: a.secret,
	}
	return token.Validate(tokenString)
}

// Middleware проверяет аутентификацию пользователя
// Поддерживает два способа передачи токена:
// 1. Cookie "token"
// 2. HTTP заголовок "Authorization: Bearer <token>"
func (a *JWTAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tokenString string
		var authMethod string

		// Проверяем cookie
		if cookie, err := r.Cookie("token"); err == nil && cookie.Value != "" {
			tokenString = cookie.Value
			authMethod = "cookie"
		}

		// Если токен не найден в cookie, проверяем заголовок Authorization
		if tokenString == "" {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					tokenString = parts[1]
					authMethod = "bearer"
				}
			}
		}

		// Если токен не найден, возвращаем 401
		if tokenString == "" {
			logger.Logger.Debugw("Authentication failed: no token provided",
				"path", r.URL.Path,
				"method", r.Method)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Валидируем токен
		login, err := a.ValidateToken(tokenString)
		if err != nil {
			logger.Logger.Debugw("Authentication failed: invalid token",
				"path", r.URL.Path,
				"method", r.Method,
				"error", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Добавляем логин пользователя в контекст
		ctx := context.WithValue(r.Context(), UserLoginKey, login)

		// TODO: Здесь можно добавить получение user_id из БД по логину
		// userID, err := storage.GetUserIDByLogin(ctx, login)
		// if err == nil {
		//     ctx = context.WithValue(ctx, UserIDKey, userID)
		// }

		logger.Logger.Debugw("User authenticated successfully",
			"login", login,
			"path", r.URL.Path,
			"method", r.Method,
			"auth_method", authMethod)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalMiddleware опциональная аутентификация (не требует обязательного наличия токена)
// Если токен предоставлен и валиден, пользователь будет аутентифицирован
// Если токен не предоставлен или невалиден, запрос продолжается без аутентификации
func (a *JWTAuth) OptionalMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tokenString string

		// Проверяем cookie
		if cookie, err := r.Cookie("token"); err == nil && cookie.Value != "" {
			tokenString = cookie.Value
		}

		// Проверяем заголовок Authorization
		if tokenString == "" {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					tokenString = parts[1]
				}
			}
		}

		// Если токен предоставлен, пытаемся валидировать
		if tokenString != "" {
			login, err := a.ValidateToken(tokenString)
			if err == nil {
				ctx := context.WithValue(r.Context(), UserLoginKey, login)
				r = r.WithContext(ctx)
				logger.Logger.Debugw("Optional auth succeeded", "login", login)
			} else {
				logger.Logger.Debugw("Optional auth failed: invalid token", "error", err)
			}
		}

		next.ServeHTTP(w, r)
	})
}

// GetUserLogin извлекает логин пользователя из контекста запроса
func GetUserLogin(ctx context.Context) string {
	if login, ok := ctx.Value(UserLoginKey).(string); ok {
		return login
	}
	return ""
}

// GetUserID извлекает ID пользователя из контекста запроса
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

// RequireAuth проверяет наличие аутентификации в контексте
// Используется в хендлерах для дополнительной проверки
func RequireAuth(ctx context.Context) (string, error) {
	login := GetUserLogin(ctx)
	if login == "" {
		return "", ErrUnauthorized
	}
	return login, nil
}

// ErrUnauthorized возвращается когда пользователь не авторизован
var ErrUnauthorized = &AuthError{Message: "unauthorized"}

// AuthError ошибка аутентификации
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}
