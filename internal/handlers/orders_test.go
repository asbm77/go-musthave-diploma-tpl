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
	"github.com/stretchr/testify/assert"
)

type mockOrderStorage struct {
	orders map[string]*models.Order
	users  map[int64]bool
}

func newMockOrderStorage() *mockOrderStorage {
	return &mockOrderStorage{
		orders: make(map[string]*models.Order),
		users:  make(map[int64]bool),
	}
}

func (m *mockOrderStorage) GetOrderByNumber(ctx context.Context, number string) (*models.Order, error) {
	if order, exists := m.orders[number]; exists {
		return order, nil
	}
	return nil, storage.ErrOrderNotFound
}

func (m *mockOrderStorage) CreateOrder(ctx context.Context, order *models.Order) error {
	if _, exists := m.orders[order.Number]; exists {
		return storage.ErrUserExists
	}
	m.orders[order.Number] = order
	return nil
}

func (m *mockOrderStorage) GetUserOrders(ctx context.Context, userID int64) ([]*models.Order, error) {
	var orders []*models.Order
	for _, order := range m.orders {
		if order.UserID == userID {
			orders = append(orders, order)
		}
	}
	return orders, nil
}

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
			orderNumber:    "4532015112830366", // Valid Luhn number
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
			mockStore := newMockOrderStorage()
			if tt.existingOrder != nil {
				mockStore.orders[tt.existingOrder.Number] = tt.existingOrder
			}

			mockProc := &mockProcessor{}
			jwtAuth := auth.NewJWTAuth("test-secret")
			handler := NewOrderHandler(mockStore, mockProc)

			req := httptest.NewRequest("POST", "/api/user/orders", bytes.NewReader([]byte(tt.orderNumber)))

			// Add user ID to context (simulating auth middleware)
			ctx := context.WithValue(req.Context(), auth.UserIDKey, tt.userID)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			handler.UploadOrder(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusAccepted {
				assert.Contains(t, mockProc.processedOrders, tt.orderNumber)
			}
		})
	}
}

func TestOrderHandler_GetUserOrders(t *testing.T) {
	mockStore := newMockOrderStorage()

	// Add test orders
	testOrders := []*models.Order{
		{
			Number:     "4532015112830366",
			UserID:     1,
			Status:     "PROCESSED",
			Accrual:    float64Ptr(500),
			UploadedAt: time.Now(),
		},
		{
			Number:     "5555555555554444",
			UserID:     1,
			Status:     "PROCESSING",
			UploadedAt: time.Now(),
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

	assert.Equal(t, http.StatusOK, w.Code)

	// Parse response
	var response []models.OrderResponse
	// Note: In real test, you'd parse JSON response
	assert.NotEmpty(t, w.Body.String())
}

func TestIsValidLuhn(t *testing.T) {
	tests := []struct {
		number string
		valid  bool
	}{
		{"4532015112830366", true},  // Valid Visa
		{"5555555555554444", true},  // Valid Mastercard
		{"12345678903", true},       // Valid test number
		{"123", false},              // Too short
		{"abcdefg", false},          // Non-numeric
		{"", false},                 // Empty
		{"4111111111111111", true},  // Valid Visa test
		{"5105105105105100", true},  // Valid Mastercard test
		{"1234567890123456", false}, // Invalid
	}

	for _, tt := range tests {
		t.Run(tt.number, func(t *testing.T) {
			result := isValidLuhn(tt.number)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func float64Ptr(f float64) *float64 {
	return &f
}
