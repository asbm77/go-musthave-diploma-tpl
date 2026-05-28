package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/stretchr/testify/assert"
)

type mockWithdrawStorage struct {
	balances    map[int64]*models.Balance
	withdrawals map[int64][]*models.Withdrawal
}

func newMockWithdrawStorage() *mockWithdrawStorage {
	return &mockWithdrawStorage{
		balances:    make(map[int64]*models.Balance),
		withdrawals: make(map[int64][]*models.Withdrawal),
	}
}

func (m *mockWithdrawStorage) WithdrawBalance(ctx context.Context, withdrawal *models.Withdrawal) error {
	balance, exists := m.balances[withdrawal.UserID]
	if !exists {
		return storage.ErrInsufficientFunds
	}

	if balance.Current < withdrawal.Sum {
		return storage.ErrInsufficientFunds
	}

	balance.Current -= withdrawal.Sum
	balance.Withdrawn += withdrawal.Sum

	m.withdrawals[withdrawal.UserID] = append(m.withdrawals[withdrawal.UserID], withdrawal)
	return nil
}

func (m *mockWithdrawStorage) GetUserWithdrawals(ctx context.Context, userID int64) ([]*models.Withdrawal, error) {
	withdrawals, exists := m.withdrawals[userID]
	if !exists {
		return []*models.Withdrawal{}, nil
	}
	return withdrawals, nil
}

func (m *mockWithdrawStorage) GetUserBalance(ctx context.Context, userID int64) (*models.Balance, error) {
	if balance, exists := m.balances[userID]; exists {
		return balance, nil
	}
	return &models.Balance{UserID: userID, Current: 0, Withdrawn: 0}, nil
}

func TestWithdrawHandler_Withdraw(t *testing.T) {
	tests := []struct {
		name           string
		userID         int64
		balance        float64
		request        models.WithdrawRequest
		expectedStatus int
	}{
		{
			name:    "successful withdrawal",
			userID:  1,
			balance: 1000,
			request: models.WithdrawRequest{
				Order: "4532015112830366",
				Sum:   100,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "insufficient funds",
			userID:  1,
			balance: 50,
			request: models.WithdrawRequest{
				Order: "4532015112830366",
				Sum:   100,
			},
			expectedStatus: http.StatusPaymentRequired,
		},
		{
			name:    "invalid order number",
			userID:  1,
			balance: 1000,
			request: models.WithdrawRequest{
				Order: "123",
				Sum:   100,
			},
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:    "negative sum",
			userID:  1,
			balance: 1000,
			request: models.WithdrawRequest{
				Order: "4532015112830366",
				Sum:   -100,
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "unauthorized user",
			userID:         0,
			balance:        1000,
			request:        models.WithdrawRequest{},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := newMockWithdrawStorage()
			mockStore.balances[tt.userID] = &models.Balance{
				UserID:    tt.userID,
				Current:   tt.balance,
				Withdrawn: 0,
			}

			handler := NewWithdrawHandler(mockStore)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/api/user/balance/withdraw", bytes.NewReader(body))

			if tt.userID != 0 {
				ctx := context.WithValue(req.Context(), auth.UserIDKey, tt.userID)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()
			handler.Withdraw(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestWithdrawHandler_GetWithdrawals(t *testing.T) {
	mockStore := newMockWithdrawStorage()
	userID := int64(1)

	// Add test withdrawals
	testWithdrawals := []*models.Withdrawal{
		{
			ID:          1,
			UserID:      userID,
			OrderNumber: "4532015112830366",
			Sum:         100,
			ProcessedAt: time.Now(),
		},
		{
			ID:          2,
			UserID:      userID,
			OrderNumber: "5555555555554444",
			Sum:         50.50,
			ProcessedAt: time.Now(),
		},
	}

	mockStore.withdrawals[userID] = testWithdrawals

	handler := NewWithdrawHandler(mockStore)

	req := httptest.NewRequest("GET", "/api/user/withdrawals", nil)
	ctx := context.WithValue(req.Context(), auth.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.GetWithdrawals(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []models.WithdrawalResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
}
