package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/stretchr/testify/assert"
)

func TestRequireAuth(t *testing.T) {
	jwtAuth := auth.NewJWTAuth("test-secret")
	userID := int64(123)

	// Generate valid token
	validToken, _ := jwtAuth.GenerateToken(userID)

	tests := []struct {
		name           string
		token          string
		expectedStatus int
	}{
		{
			name:           "valid token",
			token:          validToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "no token",
			token:          "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid token",
			token:          "invalid-token",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := RequireAuth(jwtAuth, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}

			w := httptest.NewRecorder()
			handler(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
