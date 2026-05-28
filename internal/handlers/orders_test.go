package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
)

type mockProcessor struct {
	processedOrders []string
}

func (m *mockProcessor) ProcessOrder(order *models.Order) {
	m.processedOrders = append(m.processedOrders, order.Number)
}

func TestOrderHandler_UploadOrder(t *testing.T) {
	tests := []struct {
		name           string
		orderNumber    string
		userID         int64
		existingOrder  *models.Order
		expectedStatus int
	}{
		{
			name:           "valid new order",
			orderNumber:    "4532015112830366",
			userID:         1,
			existingOrder:  nil,
			expectedStatus: http.StatusAccepted,
		},
		{
			name:           "invalid order number format",
			orderNumber:    "123",
			userID:         1,
			existingOrder:  nil,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "empty order number",
			orderNumber:    "",
			userID:         1,
			existingOrder:  nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "order already belongs to user",
			orderNumber: "4532015112830366",
			userID:      1,
			existingOrder: &models.Order{
				Number: "4532015112830366",
				UserID: 1,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "order belongs to another user",
			orderNumber: "4532015112830366",
			userID:      2,
			existingOrder: &models.Order{
				Number: "4532015112830366",
				UserID: 1,
			},
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := newMockStorage()
			if tt.existingOrder != nil {
				mockStore.orders[tt.existingOrder.Number] = tt.existingOrder
			}

			mockProc := &mockProcessor{}
			handler := NewOrderHandler(mockStore, mockProc)

			req := httptest.NewRequest("POST", "/api/user/orders", bytes.NewReader([]byte(tt.orderNumber)))

			// Add user ID to context (simulating auth middleware)
			ctx := context.WithValue(req.Context(), auth.UserIDKey, tt.userID)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			handler.UploadOrder(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusAccepted {
				found := false
				for _, processed := range mockProc.processedOrders {
					if processed == tt.orderNumber {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Order %s not processed", tt.orderNumber)
				}
			}
		})
	}
}

func TestOrderHandler_GetUserOrders(t *testing.T) {
	mockStore := newMockStorage()

	// Add test orders
	testOrders := []*models.Order{
		{
			Number:     "4532015112830366",
			UserID:     1,
			Status:     "PROCESSED",
			Accrual:    float64Ptr(500),
			UploadedAt: time.Now(),
			UpdatedAt:  time.Now(),
		},
		{
			Number:     "5555555555554444",
			UserID:     1,
			Status:     "PROCESSING",
			UploadedAt: time.Now(),
			UpdatedAt:  time.Now(),
		},
	}

	for _, order := range testOrders {
		mockStore.orders[order.Number] = order
	}

	handler := NewOrderHandler(mockStore, &mockProcessor{})

	req := httptest.NewRequest("GET", "/api/user/orders", nil)
	ctx := context.WithValue(req.Context(), auth.UserIDKey, int64(1))
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.GetUserOrders(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func float64Ptr(f float64) *float64 {
	return &f
}
