package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/stretchr/testify/assert"
)

type mockBalanceStorage struct {
	balances map[int64]*models.Balance
}

func newMockBalanceStorage() *mockBalanceStorage {
	return &mockBalanceStorage{
		balances: make(map[int64]*models.Balance),
	}
}

func (m *mockBalanceStorage) GetUserBalance(ctx context.Context, userID int64) (*models.Balance, error) {
	if balance, exists := m.balances[userID]; exists {
		return balance, nil
	}
	return &models.Balance{UserID: userID, Current: 0, Withdrawn: 0}, nil
}

func TestBalanceHandler_GetBalance(t *testing.T) {
	tests := []struct {
		name              string
		userID            int64
		setupBalance      *models.Balance
		expectedStatus    int
		expectedCurrent   float64
		expectedWithdrawn float64
	}{
		{
			name:   "user with balance",
			userID: 1,
			setupBalance: &models.Balance{
				UserID:    1,
				Current:   1000.50,
				Withdrawn: 200.25,
			},
			expectedStatus:    http.StatusOK,
			expectedCurrent:   1000.50,
			expectedWithdrawn: 200.25,
		},
		{
			name:              "user without balance",
			userID:            2,
			setupBalance:      nil,
			expectedStatus:    http.StatusOK,
			expectedCurrent:   0,
			expectedWithdrawn: 0,
		},
		{
			name:              "unauthorized user",
			userID:            0,
			setupBalance:      nil,
			expectedStatus:    http.StatusUnauthorized,
			expectedCurrent:   0,
			expectedWithdrawn: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := newMockBalanceStorage()
			if tt.setupBalance != nil {
				mockStore.balances[tt.userID] = tt.setupBalance
			}

			handler := NewBalanceHandler(mockStore)

			req := httptest.NewRequest("GET", "/api/user/balance", nil)

			if tt.userID != 0 {
				ctx := context.WithValue(req.Context(), auth.UserIDKey, tt.userID)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()
			handler.GetBalance(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response models.BalanceResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCurrent, response.Current)
				assert.Equal(t, tt.expectedWithdrawn, response.Withdrawn)
			}
		})
	}
}
