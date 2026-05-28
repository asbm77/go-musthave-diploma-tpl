package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Типы для контекста
type contextKey string

const (
	UserLoginKey contextKey = "user_login"
	UserIDKey    contextKey = "user_id"
)

// JWTAuth управляет JWT аутентификацией
type JWTAuth struct {
	secret []byte
}

// Token представляет JWT токен
type Token struct {
	Login  string
	Secret []byte
	Expiry time.Duration
}

// AuthError ошибка аутентификации
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

var ErrUnauthorized = &AuthError{Message: "unauthorized"}

// NewJWTAuth создаёт новый экземпляр JWTAuth
func NewJWTAuth(secret string) *JWTAuth {
	if secret == "" {
		secret = "default-secret-change-in-production" // должно быть переопределено через env
	}
	return &JWTAuth{
		secret: []byte(secret),
	}
}

// NewToken создаёт новый токен
func NewToken(login string, secret []byte) *Token {
	return &Token{
		Login:  login,
		Secret: secret,
		Expiry: 24 * time.Hour,
	}
}

// Generate создаёт новый JWT токен
func (t *Token) Generate() (string, error) {
	claims := jwt.MapClaims{
		"login": t.Login,
		"exp":   time.Now().Add(t.Expiry).Unix(),
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(t.Secret)
}

// Validate проверяет валидность токена и возвращает логин
func (t *Token) Validate(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return t.Secret, nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return "", fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims type")
	}

	login, ok := claims["login"].(string)
	if !ok {
		return "", fmt.Errorf("login not found in claims")
	}

	exp, ok := claims["exp"].(float64)
	if ok {
		if time.Now().Unix() > int64(exp) {
			return "", fmt.Errorf("token expired")
		}
	}

	return login, nil
}

// GenerateToken создаёт новый JWT токен для пользователя
func (a *JWTAuth) GenerateToken(login string) (string, error) {
	token := NewToken(login, a.secret)
	return token.Generate()
}

// ValidateToken проверяет валидность JWT токена
func (a *JWTAuth) ValidateToken(tokenString string) (string, error) {
	token := &Token{Secret: a.secret}
	return token.Validate(tokenString)
}

// Middleware проверяет аутентификацию
func (a *JWTAuth) Middleware(next http.Handler) http.Handler {
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

		if tokenString == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		login, err := a.ValidateToken(tokenString)
		if err != nil {
			logger.Logger.Debugw("Invalid token", "error", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserLoginKey, login)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserLogin извлекает логин из контекста
func GetUserLogin(ctx context.Context) string {
	if login, ok := ctx.Value(UserLoginKey).(string); ok {
		return login
	}
	return ""
}

// GetUserID извлекает ID пользователя из контекста
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}
