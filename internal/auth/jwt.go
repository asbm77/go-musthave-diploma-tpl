// internal/auth/jwt.go
package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTAuth struct {
	secret []byte
}

type contextKey string

const userLoginKey contextKey = "user_login"

func NewJWTAuth(secret string) *JWTAuth {
	return &JWTAuth{secret: []byte(secret)}
}

func (a *JWTAuth) GenerateToken(login string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"login": login,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
	})

	return token.SignedString(a.secret)
}

func (a *JWTAuth) ValidateToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return a.secret, nil
	})

	if err != nil {
		return "", err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		login, ok := claims["login"].(string)
		if !ok {
			return "", jwt.ErrInvalidKey
		}
		return login, nil
	}

	return "", jwt.ErrInvalidKey
}

func (a *JWTAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем cookie
		cookie, err := r.Cookie("token")
		if err == nil && cookie.Value != "" {
			login, err := a.ValidateToken(cookie.Value)
			if err == nil {
				ctx := context.WithValue(r.Context(), userLoginKey, login)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Проверяем заголовок Authorization
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				login, err := a.ValidateToken(parts[1])
				if err == nil {
					ctx := context.WithValue(r.Context(), userLoginKey, login)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

func GetUserLogin(ctx context.Context) string {
	if login, ok := ctx.Value(userLoginKey).(string); ok {
		return login
	}
	return ""
}
