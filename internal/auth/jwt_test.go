package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJWTAuth(t *testing.T) {
	secret := "test-secret"
	jwtAuth := NewJWTAuth(secret)

	assert.NotNil(t, jwtAuth)
	assert.Equal(t, []byte(secret), jwtAuth.secretKey)
}

func TestJWTAuth_GenerateToken(t *testing.T) {
	tests := []struct {
		name    string
		userID  int64
		wantErr bool
	}{
		{
			name:    "valid user ID",
			userID:  123,
			wantErr: false,
		},
		{
			name:    "zero user ID",
			userID:  0,
			wantErr: false,
		},
		{
			name:    "negative user ID",
			userID:  -1,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jwtAuth := NewJWTAuth("test-secret")
			token, err := jwtAuth.GenerateToken(tt.userID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, token)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, token)
			}
		})
	}
}

func TestJWTAuth_ValidateToken(t *testing.T) {
	jwtAuth := NewJWTAuth("test-secret")
	userID := int64(456)

	token, err := jwtAuth.GenerateToken(userID)
	require.NoError(t, err)

	tests := []struct {
		name       string
		setupReq   func() *http.Request
		wantUserID int64
		wantErr    bool
	}{
		{
			name: "valid token in Authorization header",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "/", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			wantUserID: userID,
			wantErr:    false,
		},
		{
			name: "valid token in cookie",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "/", nil)
				req.AddCookie(&http.Cookie{Name: "token", Value: token})
				return req
			},
			wantUserID: userID,
			wantErr:    false,
		},
		{
			name: "no token",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "/", nil)
				return req
			},
			wantUserID: 0,
			wantErr:    true,
		},
		{
			name: "invalid token",
			setupReq: func() *http.Request {
				req, _ := http.NewRequest("GET", "/", nil)
				req.Header.Set("Authorization", "Bearer invalid-token")
				return req
			},
			wantUserID: 0,
			wantErr:    true,
		},
		{
			name: "expired token",
			setupReq: func() *http.Request {
				// Create expired token
				claims := jwt.MapClaims{
					"user_id": userID,
					"exp":     time.Now().Add(-time.Hour).Unix(),
				}
				expiredToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				tokenStr, _ := expiredToken.SignedString([]byte("test-secret"))

				req, _ := http.NewRequest("GET", "/", nil)
				req.Header.Set("Authorization", "Bearer "+tokenStr)
				return req
			},
			wantUserID: 0,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupReq()
			gotUserID, err := jwtAuth.ValidateToken(req)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantUserID, gotUserID)
			}
		})
	}
}

func TestGetUserIDFromContext(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		want    int64
		wantErr bool
	}{
		{
			name:    "valid user ID in context",
			ctx:     context.WithValue(context.Background(), UserIDKey, int64(123)),
			want:    123,
			wantErr: false,
		},
		{
			name:    "no user ID in context",
			ctx:     context.Background(),
			want:    0,
			wantErr: true,
		},
		{
			name:    "wrong type in context",
			ctx:     context.WithValue(context.Background(), UserIDKey, "not-int"),
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetUserIDFromContext(tt.ctx)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
