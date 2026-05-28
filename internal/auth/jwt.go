package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserIDKey contextKey = "userID"

// JWTAuth управляет JWT аутентификацией
type JWTAuth struct {
	secretKey []byte
}

// NewJWTAuth создаёт новый экземпляр JWTAuth
func NewJWTAuth(secret string) *JWTAuth {
	return &JWTAuth{
		secretKey: []byte(secret),
	}
}

// GenerateToken генерирует JWT токен для пользователя
func (j *JWTAuth) GenerateToken(userID int64) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secretKey)
}

// ValidateToken проверяет JWT токен и возвращает userID
func (j *JWTAuth) ValidateToken(r *http.Request) (int64, error) {
	// Получаем токен из заголовка Authorization
	tokenString := r.Header.Get("Authorization")
	if tokenString == "" {
		// Пробуем получить из cookie
		cookie, err := r.Cookie("token")
		if err == nil {
			tokenString = cookie.Value
		}
	}

	if tokenString == "" {
		return 0, errors.New("no token provided")
	}

	// Убираем "Bearer " если есть
	if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
		tokenString = tokenString[7:]
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return j.secretKey, nil
	})

	if err != nil {
		return 0, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userIDFloat, ok := claims["user_id"].(float64)
		if !ok {
			return 0, errors.New("invalid user_id in token")
		}
		return int64(userIDFloat), nil
	}

	return 0, errors.New("invalid token")
}

// GetUserIDFromContext получает userID из контекста запроса
func GetUserIDFromContext(ctx context.Context) (int64, error) {
	userID, ok := ctx.Value(UserIDKey).(int64)
	if !ok {
		return 0, errors.New("user ID not found in context")
	}
	return userID, nil
}
