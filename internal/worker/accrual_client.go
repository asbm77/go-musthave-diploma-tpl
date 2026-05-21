// internal/worker/accrual_client.go
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

type AccrualClient struct {
	baseURL    string
	httpClient *http.Client
	limiter    *rate.Limiter // Rate limiter для соблюдения ограничений
}

func NewAccrualClient(baseURL string) *AccrualClient {
	return &AccrualClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		limiter: rate.NewLimiter(rate.Limit(10), 20), // 10 запросов в секунду
	}
}

func (c *AccrualClient) GetAccrual(ctx context.Context, orderNumber string) (*storage.AccrualResponse, error) {
	// Ждём разрешения от rate limiter
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/orders/%s", c.baseURL, orderNumber)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Retry logic
	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt < 5; attempt++ {
		resp, err = c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			var accrualResp storage.AccrualResponse
			if err := json.NewDecoder(resp.Body).Decode(&accrualResp); err != nil {
				return nil, err
			}
			return &accrualResp, nil

		case http.StatusNoContent:
			return &storage.AccrualResponse{
				Order:  orderNumber,
				Status: "REGISTERED",
			}, nil

		case http.StatusTooManyRequests:
			// Получаем Retry-After
			retryAfter := resp.Header.Get("Retry-After")
			waitTime := 60 * time.Second
			if retryAfter != "" {
				if seconds, err := time.ParseDuration(retryAfter + "s"); err == nil {
					waitTime = seconds
				}
			}
			logger.Logger.Warnw("Rate limited by accrual system", "retry_after", waitTime)
			time.Sleep(waitTime)
			continue

		default:
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}
