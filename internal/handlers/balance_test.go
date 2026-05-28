package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
)

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
				UpdatedAt: time.Now(),
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := newMockStorage()
			if tt.setupBalance != nil {
				mockStore.balances[tt.userID] = tt.setupBalance
			}

			handler := NewBalanceHandler(mockStore)

			req := httptest.NewRequest("GET", "/api/user/balance", nil)
			ctx := context.WithValue(req.Context(), auth.UserIDKey, tt.userID)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			handler.GetBalance(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var response models.BalanceResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				if err != nil {
					t.Errorf("Failed to decode response: %v", err)
				}
				if response.Current != tt.expectedCurrent {
					t.Errorf("Expected current %f, got %f", tt.expectedCurrent, response.Current)
				}
				if response.Withdrawn != tt.expectedWithdrawn {
					t.Errorf("Expected withdrawn %f, got %f", tt.expectedWithdrawn, response.Withdrawn)
				}
			}
		})
	}
}
