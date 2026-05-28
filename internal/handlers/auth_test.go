package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/storage"
)

// mockStorage реализует интерфейс storage.StorageInterface для тестов
type mockStorage struct {
	users       map[string]*models.User
	orders      map[string]*models.Order
	balances    map[int64]*models.Balance
	withdrawals map[int64][]*models.Withdrawal
	nextUserID  int64
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		users:       make(map[string]*models.User),
		orders:      make(map[string]*models.Order),
		balances:    make(map[int64]*models.Balance),
		withdrawals: make(map[int64][]*models.Withdrawal),
		nextUserID:  1,
	}
}

func (m *mockStorage) CreateUser(ctx context.Context, login, password string) (*models.User, error) {
	if _, exists := m.users[login]; exists {
		return nil, storage.ErrUserExists
	}
	user := &models.User{
		ID:        m.nextUserID,
		Login:     login,
		Password:  password,
		CreatedAt: time.Now(),
	}
	m.users[login] = user
	m.balances[user.ID] = &models.Balance{
		UserID:    user.ID,
		Current:   0,
		Withdrawn: 0,
		UpdatedAt: time.Now(),
	}
	m.nextUserID++
	return user, nil
}

func (m *mockStorage) GetUserByLogin(ctx context.Context, login string) (*models.User, error) {
	user, exists := m.users[login]
	if !exists {
		return nil, storage.ErrUserNotFound
	}
	return user, nil
}

func (m *mockStorage) GetOrderByNumber(ctx context.Context, number string) (*models.Order, error) {
	order, exists := m.orders[number]
	if !exists {
		return nil, storage.ErrOrderNotFound
	}
	return order, nil
}

func (m *mockStorage) CreateOrder(ctx context.Context, order *models.Order) error {
	if _, exists := m.orders[order.Number]; exists {
		return storage.ErrUserExists
	}
	m.orders[order.Number] = order
	return nil
}

func (m *mockStorage) GetUserOrders(ctx context.Context, userID int64) ([]*models.Order, error) {
	var orders []*models.Order
	for _, order := range m.orders {
		if order.UserID == userID {
			orders = append(orders, order)
		}
	}
	return orders, nil
}

func (m *mockStorage) UpdateOrderStatus(ctx context.Context, number string, status string, accrual *float64) error {
	if order, exists := m.orders[number]; exists {
		order.Status = status
		order.Accrual = accrual
		order.UpdatedAt = time.Now()
	}
	return nil
}

func (m *mockStorage) GetUserBalance(ctx context.Context, userID int64) (*models.Balance, error) {
	balance, exists := m.balances[userID]
	if !exists {
		return &models.Balance{UserID: userID, Current: 0, Withdrawn: 0, UpdatedAt: time.Now()}, nil
	}
	return balance, nil
}

func (m *mockStorage) WithdrawBalance(ctx context.Context, withdrawal *models.Withdrawal) error {
	balance, exists := m.balances[withdrawal.UserID]
	if !exists || balance.Current < withdrawal.Sum {
		return storage.ErrInsufficientFunds
	}
	balance.Current -= withdrawal.Sum
	balance.Withdrawn += withdrawal.Sum
	balance.UpdatedAt = time.Now()
	m.withdrawals[withdrawal.UserID] = append(m.withdrawals[withdrawal.UserID], withdrawal)
	return nil
}

func (m *mockStorage) GetUserWithdrawals(ctx context.Context, userID int64) ([]*models.Withdrawal, error) {
	withdrawals, exists := m.withdrawals[userID]
	if !exists {
		return []*models.Withdrawal{}, nil
	}
	return withdrawals, nil
}

func (m *mockStorage) GetPendingOrders(ctx context.Context) ([]*models.Order, error) {
	var orders []*models.Order
	for _, order := range m.orders {
		if order.Status == "NEW" || order.Status == "PROCESSING" {
			orders = append(orders, order)
		}
	}
	return orders, nil
}

func (m *mockStorage) Close() error {
	return nil
}

func TestAuthHandler_Register(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(*mockStorage)
		requestBody    interface{}
		expectedStatus int
	}{
		{
			name: "successful registration",
			setupMock: func(m *mockStorage) {
				// no setup needed
			},
			requestBody: models.RegisterRequest{
				Login:    "testuser",
				Password: "password123",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing login",
			setupMock: func(m *mockStorage) {
				// no setup needed
			},
			requestBody: models.RegisterRequest{
				Password: "password123",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing password",
			setupMock: func(m *mockStorage) {
				// no setup needed
			},
			requestBody: models.RegisterRequest{
				Login: "testuser",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty request body",
			setupMock: func(m *mockStorage) {
				// no setup needed
			},
			requestBody:    nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate login",
			setupMock: func(m *mockStorage) {
				m.users["duplicate"] = &models.User{ID: 1, Login: "duplicate"}
			},
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
			if tt.setupMock != nil {
				tt.setupMock(mockStore)
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

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				if w.Header().Get("Authorization") == "" {
					t.Error("Expected Authorization header")
				}
				cookies := w.Result().Cookies()
				if len(cookies) == 0 {
					t.Error("Expected cookie")
				}
			}
		})
	}
}

func TestAuthHandler_Login(t *testing.T) {
	mockStore := newMockStorage()

	// Create a test user with hashed password
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	mockStore.users["testuser"] = &models.User{
		ID:        1,
		Login:     "testuser",
		Password:  string(hashedPassword),
		CreatedAt: time.Now(),
	}

	jwtAuth := auth.NewJWTAuth("test-secret")
	handler := NewAuthHandler(mockStore, jwtAuth)

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
		{
			name: "empty password",
			requestBody: models.LoginRequest{
				Login: "testuser",
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

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				if w.Header().Get("Authorization") == "" {
					t.Error("Expected Authorization header")
				}
			}
		})
	}
}
