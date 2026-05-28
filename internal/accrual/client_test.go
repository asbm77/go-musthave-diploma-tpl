package accrual

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	baseURL := "http://test-accrual.com"
	client := NewClient(baseURL)

	assert.NotNil(t, client)
	assert.Equal(t, baseURL, client.baseURL)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, 10*time.Second, client.httpClient.Timeout)
}

func TestClient_GetOrderInfo(t *testing.T) {
	tests := []struct {
		name           string
		orderNumber    string
		statusCode     int
		responseBody   interface{}
		expectedResult *models.AccrualOrderResponse
		expectedError  bool
	}{
		{
			name:        "successful response - PROCESSED",
			orderNumber: "12345678903",
			statusCode:  http.StatusOK,
			responseBody: models.AccrualOrderResponse{
				Order:   "12345678903",
				Status:  "PROCESSED",
				Accrual: 500,
			},
			expectedResult: &models.AccrualOrderResponse{
				Order:   "12345678903",
				Status:  "PROCESSED",
				Accrual: 500,
			},
			expectedError: false,
		},
		{
			name:        "successful response - PROCESSING",
			orderNumber: "5555555555554444",
			statusCode:  http.StatusOK,
			responseBody: models.AccrualOrderResponse{
				Order:  "5555555555554444",
				Status: "PROCESSING",
			},
			expectedResult: &models.AccrualOrderResponse{
				Order:  "5555555555554444",
				Status: "PROCESSING",
			},
			expectedError: false,
		},
		{
			name:        "successful response - INVALID",
			orderNumber: "1111111111111111",
			statusCode:  http.StatusOK,
			responseBody: models.AccrualOrderResponse{
				Order:  "1111111111111111",
				Status: "INVALID",
			},
			expectedResult: &models.AccrualOrderResponse{
				Order:  "1111111111111111",
				Status: "INVALID",
			},
			expectedError: false,
		},
		{
			name:           "order not registered",
			orderNumber:    "9999999999999999",
			statusCode:     http.StatusNoContent,
			responseBody:   nil,
			expectedResult: nil,
			expectedError:  false,
		},
		{
			name:           "too many requests",
			orderNumber:    "12345678903",
			statusCode:     http.StatusTooManyRequests,
			responseBody:   nil,
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name:           "internal server error",
			orderNumber:    "12345678903",
			statusCode:     http.StatusInternalServerError,
			responseBody:   nil,
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name:           "malformed JSON response",
			orderNumber:    "12345678903",
			statusCode:     http.StatusOK,
			responseBody:   "invalid json",
			expectedResult: nil,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				assert.Equal(t, "GET", r.Method)
				assert.Contains(t, r.URL.Path, tt.orderNumber)

				w.WriteHeader(tt.statusCode)

				if tt.responseBody != nil {
					var respBody []byte
					var err error

					switch v := tt.responseBody.(type) {
					case models.AccrualOrderResponse:
						respBody, err = json.Marshal(v)
						require.NoError(t, err)
					case string:
						respBody = []byte(v)
					}

					_, err = w.Write(respBody)
					require.NoError(t, err)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL)
			ctx := context.Background()

			result, err := client.GetOrderInfo(ctx, tt.orderNumber)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestClient_GetOrderInfoWithRetry(t *testing.T) {
	tests := []struct {
		name           string
		orderNumber    string
		responses      []int // status codes for each attempt
		expectedResult *models.AccrualOrderResponse
		expectedError  bool
	}{
		{
			name:        "success on first attempt",
			orderNumber: "12345678903",
			responses:   []int{http.StatusOK},
			expectedResult: &models.AccrualOrderResponse{
				Order:   "12345678903",
				Status:  "PROCESSED",
				Accrual: 500,
			},
			expectedError: false,
		},
		{
			name:        "success after retry",
			orderNumber: "12345678903",
			responses:   []int{http.StatusTooManyRequests, http.StatusOK},
			expectedResult: &models.AccrualOrderResponse{
				Order:   "12345678903",
				Status:  "PROCESSED",
				Accrual: 500,
			},
			expectedError: false,
		},
		{
			name:           "all attempts fail",
			orderNumber:    "12345678903",
			responses:      []int{http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests},
			expectedResult: nil,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempt := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if attempt < len(tt.responses) {
					w.WriteHeader(tt.responses[attempt])
					attempt++

					if tt.responses[attempt-1] == http.StatusOK {
						resp := models.AccrualOrderResponse{
							Order:   tt.orderNumber,
							Status:  "PROCESSED",
							Accrual: 500,
						}
						json.NewEncoder(w).Encode(resp)
					}
				}
			}))
			defer server.Close()

			client := NewClient(server.URL)
			ctx := context.Background()

			result, err := client.GetOrderInfoWithRetry(ctx, tt.orderNumber, 3, 100*time.Millisecond)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestClient_GetOrderStatus(t *testing.T) {
	tests := []struct {
		name           string
		orderNumber    string
		responseStatus int
		responseBody   interface{}
		expectedStatus string
		expectedError  bool
	}{
		{
			name:           "status PROCESSED",
			orderNumber:    "12345678903",
			responseStatus: http.StatusOK,
			responseBody: models.AccrualOrderResponse{
				Order:  "12345678903",
				Status: "PROCESSED",
			},
			expectedStatus: "PROCESSED",
			expectedError:  false,
		},
		{
			name:           "status PROCESSING",
			orderNumber:    "5555555555554444",
			responseStatus: http.StatusOK,
			responseBody: models.AccrualOrderResponse{
				Order:  "5555555555554444",
				Status: "PROCESSING",
			},
			expectedStatus: "PROCESSING",
			expectedError:  false,
		},
		{
			name:           "status INVALID",
			orderNumber:    "1111111111111111",
			responseStatus: http.StatusOK,
			responseBody: models.AccrualOrderResponse{
				Order:  "1111111111111111",
				Status: "INVALID",
			},
			expectedStatus: "INVALID",
			expectedError:  false,
		},
		{
			name:           "order not registered - returns REGISTERED",
			orderNumber:    "9999999999999999",
			responseStatus: http.StatusNoContent,
			responseBody:   nil,
			expectedStatus: "REGISTERED",
			expectedError:  false,
		},
		{
			name:           "server error",
			orderNumber:    "12345678903",
			responseStatus: http.StatusInternalServerError,
			responseBody:   nil,
			expectedStatus: "",
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.responseStatus)
				if tt.responseBody != nil {
					json.NewEncoder(w).Encode(tt.responseBody)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL)
			ctx := context.Background()

			status, err := client.GetOrderStatus(ctx, tt.orderNumber)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, status)
			}
		})
	}
}

func TestClient_IsOrderProcessed(t *testing.T) {
	tests := []struct {
		name           string
		orderNumber    string
		responseStatus int
		responseBody   interface{}
		expectedResult bool
		expectedError  bool
	}{
		{
			name:           "PROCESSED - returns true",
			orderNumber:    "12345678903",
			responseStatus: http.StatusOK,
			responseBody: models.AccrualOrderResponse{
				Order:  "12345678903",
				Status: "PROCESSED",
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name:           "INVALID - returns true",
			orderNumber:    "1111111111111111",
			responseStatus: http.StatusOK,
			responseBody: models.AccrualOrderResponse{
				Order:  "1111111111111111",
				Status: "INVALID",
			},
			expectedResult: true,
			expectedError:  false,
		},
		{
			name:           "PROCESSING - returns false",
			orderNumber:    "5555555555554444",
			responseStatus: http.StatusOK,
			responseBody: models.AccrualOrderResponse{
				Order:  "5555555555554444",
				Status: "PROCESSING",
			},
			expectedResult: false,
			expectedError:  false,
		},
		{
			name:           "not registered - returns false",
			orderNumber:    "9999999999999999",
			responseStatus: http.StatusNoContent,
			responseBody:   nil,
			expectedResult: false,
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.responseStatus)
				if tt.responseBody != nil {
					json.NewEncoder(w).Encode(tt.responseBody)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL)
			ctx := context.Background()

			result, err := client.IsOrderProcessed(ctx, tt.orderNumber)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestClient_GetAccrual(t *testing.T) {
	tests := []struct {
		name            string
		orderNumber     string
		responseStatus  int
		responseBody    interface{}
		expectedAccrual float64
		expectedError   bool
	}{
		{
			name:           "has accrual",
			orderNumber:    "12345678903",
			responseStatus: http.StatusOK,
			responseBody: models.AccrualOrderResponse{
				Order:   "12345678903",
				Status:  "PROCESSED",
				Accrual: 500,
			},
			expectedAccrual: 500,
			expectedError:   false,
		},
		{
			name:           "zero accrual",
			orderNumber:    "5555555555554444",
			responseStatus: http.StatusOK,
			responseBody: models.AccrualOrderResponse{
				Order:   "5555555555554444",
				Status:  "PROCESSING",
				Accrual: 0,
			},
			expectedAccrual: 0,
			expectedError:   false,
		},
		{
			name:            "order not registered",
			orderNumber:     "9999999999999999",
			responseStatus:  http.StatusNoContent,
			responseBody:    nil,
			expectedAccrual: 0,
			expectedError:   false,
		},
		{
			name:            "server error",
			orderNumber:     "12345678903",
			responseStatus:  http.StatusInternalServerError,
			responseBody:    nil,
			expectedAccrual: 0,
			expectedError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.responseStatus)
				if tt.responseBody != nil {
					json.NewEncoder(w).Encode(tt.responseBody)
				}
			}))
			defer server.Close()

			client := NewClient(server.URL)
			ctx := context.Background()

			accrual, err := client.GetAccrual(ctx, tt.orderNumber)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAccrual, accrual)
			}
		})
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.GetOrderInfo(ctx, "12345678903")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestClient_RetryWithContextCancellation(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := client.GetOrderInfoWithRetry(ctx, "12345678903", 10, 200*time.Millisecond)
	assert.Error(t, err)
	// Should not make all attempts because context will cancel
	assert.Less(t, attempts, 5)
}
