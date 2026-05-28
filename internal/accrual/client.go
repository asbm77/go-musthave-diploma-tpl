package accrual

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/asbm77/go-musthave-diploma-tpl/pkg/logger"
)

// Client представляет клиент для взаимодействия с системой расчёта баллов
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient создаёт новый экземпляр клиента системы расчёта баллов
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetOrderInfo запрашивает информацию о заказе в системе расчёта баллов
// GET /api/orders/{number}
func (c *Client) GetOrderInfo(ctx context.Context, orderNumber string) (*models.AccrualOrderResponse, error) {
	url := fmt.Sprintf("%s/api/orders/%s", c.baseURL, orderNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Logger.Errorw("Failed to call accrual system", "error", err, "order", orderNumber)
		return nil, fmt.Errorf("failed to call accrual system: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var result models.AccrualOrderResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return &result, nil

	case http.StatusNoContent:
		// Заказ не зарегистрирован в системе
		return nil, nil

	case http.StatusTooManyRequests:
		// Превышен лимит запросов
		retryAfter := resp.Header.Get("Retry-After")
		logger.Logger.Warnw("Rate limited by accrual system", "order", orderNumber, "retry_after", retryAfter)
		return nil, fmt.Errorf("rate limited by accrual system")

	default:
		logger.Logger.Errorw("Unexpected status code from accrual system",
			"status_code", resp.StatusCode,
			"order", orderNumber)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

// GetOrderInfoWithRetry запрашивает информацию о заказе с повторными попытками
func (c *Client) GetOrderInfoWithRetry(ctx context.Context, orderNumber string, maxRetries int, retryDelay time.Duration) (*models.AccrualOrderResponse, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := c.GetOrderInfo(ctx, orderNumber)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Если ошибка из-за rate limiting, ждём дольше
		if err.Error() == "rate limited by accrual system" {
			time.Sleep(retryDelay * 2)
		} else {
			time.Sleep(retryDelay)
		}
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// GetOrderStatus возвращает только статус заказа из системы расчёта
func (c *Client) GetOrderStatus(ctx context.Context, orderNumber string) (string, error) {
	info, err := c.GetOrderInfo(ctx, orderNumber)
	if err != nil {
		return "", err
	}
	if info == nil {
		return "REGISTERED", nil // Заказ зарегистрирован, но ещё не обработан
	}
	return info.Status, nil
}

// IsOrderProcessed проверяет, обработан ли заказ (финальный статус)
func (c *Client) IsOrderProcessed(ctx context.Context, orderNumber string) (bool, error) {
	info, err := c.GetOrderInfo(ctx, orderNumber)
	if err != nil {
		return false, err
	}
	if info == nil {
		return false, nil
	}
	return info.Status == "PROCESSED" || info.Status == "INVALID", nil
}

// GetAccrual возвращает сумму начисления для заказа
func (c *Client) GetAccrual(ctx context.Context, orderNumber string) (float64, error) {
	info, err := c.GetOrderInfo(ctx, orderNumber)
	if err != nil {
		return 0, err
	}
	if info == nil {
		return 0, nil
	}
	return info.Accrual, nil
}
