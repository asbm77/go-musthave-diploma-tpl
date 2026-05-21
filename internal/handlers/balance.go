// internal/handlers/balance.go
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// GetBalance возвращает текущий баланс пользователя
// GET /api/user/balance
func GetBalance(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Проверяем аутентификацию
		login := auth.GetUserLogin(r.Context())
		if login == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Получаем userID по логину
		userID, err := store.GetUserIDByLogin(r.Context(), login)
		if err != nil {
			logger.Logger.Errorw("Failed to get user ID",
				"login", login,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Получаем баланс
		balance, err := store.GetBalance(r.Context(), userID)
		if err != nil {
			logger.Logger.Errorw("Failed to get balance",
				"user_id", userID,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Формируем ответ
		response := BalanceResponse{
			Current:   balance.Current,
			Withdrawn: balance.Withdrawn,
		}

		// Отправляем ответ
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Logger.Errorw("Failed to encode response",
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logger.Logger.Debugw("User balance retrieved",
			"user_id", userID,
			"current", balance.Current,
			"withdrawn", balance.Withdrawn)
	}
}

// Withdraw обрабатывает запрос на списание баллов
// POST /api/user/balance/withdraw
func Withdraw(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Проверяем аутентификацию
		login := auth.GetUserLogin(r.Context())
		if login == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Декодируем запрос
		var req WithdrawRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Logger.Debugw("Invalid withdraw request format",
				"error", err)
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}

		// Валидация
		if req.Order == "" {
			http.Error(w, "Order number is required", http.StatusBadRequest)
			return
		}

		if req.Sum <= 0 {
			http.Error(w, "Withdrawal sum must be positive", http.StatusBadRequest)
			return
		}

		// Проверяем номер заказа по алгоритму Луна
		if !utils.ValidateLuhn(req.Order) {
			logger.Logger.Debugw("Invalid order number format for withdrawal",
				"order", req.Order)
			http.Error(w, "Invalid order number format", http.StatusUnprocessableEntity)
			return
		}

		// Получаем userID по логину
		userID, err := store.GetUserIDByLogin(r.Context(), login)
		if err != nil {
			logger.Logger.Errorw("Failed to get user ID",
				"login", login,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Проверяем баланс и списываем средства
		err = store.Withdraw(r.Context(), userID, req.Order, req.Sum)
		if err != nil {
			switch err {
			case storage.ErrInsufficientFunds:
				logger.Logger.Debugw("Insufficient funds for withdrawal",
					"user_id", userID,
					"requested_sum", req.Sum)
				http.Error(w, "Insufficient funds", http.StatusPaymentRequired)
				return
			default:
				logger.Logger.Errorw("Failed to process withdrawal",
					"user_id", userID,
					"order", req.Order,
					"sum", req.Sum,
					"error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		// Успешное списание
		logger.Logger.Infow("Withdrawal processed successfully",
			"user_id", userID,
			"order", req.Order,
			"sum", req.Sum)

		w.WriteHeader(http.StatusOK)
	}
}

// GetWithdrawals возвращает историю выводов средств
// GET /api/user/withdrawals
func GetWithdrawals(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Проверяем аутентификацию
		login := auth.GetUserLogin(r.Context())
		if login == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Получаем userID по логину
		userID, err := store.GetUserIDByLogin(r.Context(), login)
		if err != nil {
			logger.Logger.Errorw("Failed to get user ID",
				"login", login,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Получаем историю выводов
		withdrawals, err := store.GetWithdrawals(r.Context(), userID)
		if err != nil {
			logger.Logger.Errorw("Failed to get withdrawals",
				"user_id", userID,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Если выводов нет, возвращаем 204 No Content
		if len(withdrawals) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Формируем ответ
		response := make([]WithdrawalResponse, 0, len(withdrawals))
		for _, wd := range withdrawals {
			response = append(response, WithdrawalResponse{
				Order:       wd.OrderNumber,
				Sum:         wd.Sum,
				ProcessedAt: wd.ProcessedAt.Format(time.RFC3339),
			})
		}

		// Отправляем ответ
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Logger.Errorw("Failed to encode response",
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logger.Logger.Debugw("Withdrawals history retrieved",
			"user_id", userID,
			"count", len(response))
	}
}

// GetBalanceHistory возвращает историю изменений баланса (дополнительный эндпоинт)
// GET /api/user/balance/history
func GetBalanceHistory(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Проверяем аутентификацию
		login := auth.GetUserLogin(r.Context())
		if login == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Получаем userID по логину
		userID, err := store.GetUserIDByLogin(r.Context(), login)
		if err != nil {
			logger.Logger.Errorw("Failed to get user ID",
				"login", login,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Параметры пагинации
		limit := 50
		offset := 0

		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if l, err := parseInt(limitStr); err == nil && l > 0 && l <= 100 {
				limit = l
			}
		}

		if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
			if o, err := parseInt(offsetStr); err == nil && o >= 0 {
				offset = o
			}
		}

		// Получаем историю баланса
		history, err := store.GetBalanceHistory(r.Context(), userID, limit, offset)
		if err != nil {
			logger.Logger.Errorw("Failed to get balance history",
				"user_id", userID,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Если истории нет, возвращаем 204 No Content
		if len(history) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Формируем ответ
		response := make([]BalanceHistoryResponse, 0, len(history))
		for _, entry := range history {
			response = append(response, BalanceHistoryResponse{
				Amount:      entry.Amount,
				Type:        entry.Type,
				Description: entry.Description,
				OrderNumber: entry.OrderNumber,
				CreatedAt:   entry.CreatedAt.Format(time.RFC3339),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// BalanceResponse представляет ответ с балансом пользователя
type BalanceResponse struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

// WithdrawRequest представляет запрос на списание средств
type WithdrawRequest struct {
	Order string  `json:"order"`
	Sum   float64 `json:"sum"`
}

// WithdrawalResponse представляет ответ с информацией о выводе
type WithdrawalResponse struct {
	Order       string  `json:"order"`
	Sum         float64 `json:"sum"`
	ProcessedAt string  `json:"processed_at"`
}

// BalanceHistoryResponse представляет запись истории баланса
type BalanceHistoryResponse struct {
	Amount      float64 `json:"amount"`
	Type        string  `json:"type"` // "accrual" или "withdrawal"
	Description string  `json:"description,omitempty"`
	OrderNumber string  `json:"order_number,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

// parseInt парсит строку в int
func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}
