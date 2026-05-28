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
	"github.com/asbm77/go-musthave-diploma-tpl/internal/storage"
)

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := newMockStorage()
			mockStore.balances[tt.userID] = &models.Balance{
				UserID:    tt.userID,
				Current:   tt.balance,
				Withdrawn: 0,
				UpdatedAt: time.Now(),
			}

			handler := NewWithdrawHandler(mockStore)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/api/user/balance/withdraw", bytes.NewReader(body))
			ctx := context.WithValue(req.Context(), auth.UserIDKey, tt.userID)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			handler.Withdraw(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestWithdrawHandler_GetWithdrawals(t *testing.T) {
	mockStore := newMockStorage()
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

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response []models.WithdrawalResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	if err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	if len(response) != 2 {
		t.Errorf("Expected 2 withdrawals, got %d", len(response))
	}
}
