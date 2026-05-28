package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStorage struct {
	users map[string]*models.User
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		users: make(map[string]*models.User),
	}
}

func (m *mockStorage) CreateUser(ctx context.Context, login, password string) (*models.User, error) {
	if _, exists := m.users[login]; exists {
		return nil, storage.ErrUserExists
	}
	user := &models.User{
		ID:       int64(len(m.users) + 1),
		Login:    login,
		Password: password,
	}
	m.users[login] = user
	return user, nil
}

func (m *mockStorage) GetUserByLogin(ctx context.Context, login string) (*models.User, error) {
	user, exists := m.users[login]
	if !exists {
		return nil, storage.ErrUserNotFound
	}
	return user, nil
}

func TestAuthHandler_Register(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
	}{
		{
			name: "successful registration",
			requestBody: models.RegisterRequest{
				Login:    "testuser",
				Password: "password123",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing login",
			requestBody: models.RegisterRequest{
				Password: "password123",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing password",
			requestBody: models.RegisterRequest{
				Login: "testuser",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty request body",
			requestBody:    nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate login",
			requestBody: models.RegisterRequest{
				Login:    "duplicate",
				Password: "password123",
			},
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := newMockStorage()
			// Add duplicate user for the duplicate test
			if tt.name == "duplicate login" {
				mockStore.users["duplicate"] = &models.User{ID: 1, Login: "duplicate"}
			}

			jwtAuth := auth.NewJWTAuth("test-secret")
			handler := NewAuthHandler(mockStore, jwtAuth)

			var body []byte
			if tt.requestBody != nil {
				body, _ = json.Marshal(tt.requestBody)
			}

			req := httptest.NewRequest("POST", "/api/user/register", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler.Register(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.NotEmpty(t, w.Header().Get("Authorization"))
				cookies := w.Result().Cookies()
				assert.NotEmpty(t, cookies)
			}
		})
	}
}

func TestAuthHandler_Login(t *testing.T) {
	mockStore := newMockStorage()
	jwtAuth := auth.NewJWTAuth("test-secret")
	handler := NewAuthHandler(mockStore, jwtAuth)

	// Create a test user
	mockStore.users["testuser"] = &models.User{
		ID:       1,
		Login:    "testuser",
		Password: "$2a$10$testhash", // This would be a real hash in production
	}

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
	}{
		{
			name: "valid credentials",
			requestBody: models.LoginRequest{
				Login:    "testuser",
				Password: "password123",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid password",
			requestBody: models.LoginRequest{
				Login:    "testuser",
				Password: "wrongpassword",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "non-existent user",
			requestBody: models.LoginRequest{
				Login:    "nonexistent",
				Password: "password",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "empty login",
			requestBody: models.LoginRequest{
				Password: "password",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/api/user/login", bytes.NewReader(body))
			w := httptest.NewRecorder()

			handler.Login(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.NotEmpty(t, w.Header().Get("Authorization"))
			}
		})
	}
}
