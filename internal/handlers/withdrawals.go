// internal/handlers/withdrawals.go
package handlers

import (
	"encoding/json"
	"net/http"
	"time"
)

// GetWithdrawals возвращает историю выводов средств пользователя
// GET /api/user/withdrawals
func GetWithdrawals(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Проверяем аутентификацию
		login := auth.GetUserLogin(r.Context())
		if login == "" {
			logger.Logger.Debugw("Unauthorized access to withdrawals",
				"remote_addr", r.RemoteAddr)
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
			logger.Logger.Debugw("No withdrawals found for user",
				"user_id", userID)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Формируем ответ согласно спецификации
		// Сортировка уже выполнена в БД (ORDER BY processed_at DESC)
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
			logger.Logger.Errorw("Failed to encode withdrawals response",
				"user_id", userID,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logger.Logger.Debugw("Withdrawals history retrieved successfully",
			"user_id", userID,
			"count", len(response))
	}
}

// GetWithdrawalsSummary возвращает сводку по выводам пользователя (дополнительный эндпоинт)
// GET /api/user/withdrawals/summary
func GetWithdrawalsSummary(store storage.Storage) http.HandlerFunc {
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

		// Получаем сводку
		summary, err := store.GetWithdrawalsSummary(r.Context(), userID)
		if err != nil {
			logger.Logger.Errorw("Failed to get withdrawals summary",
				"user_id", userID,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Формируем ответ
		response := WithdrawalsSummaryResponse{
			TotalWithdrawn:    summary.TotalWithdrawn,
			WithdrawalsCount:  summary.WithdrawalsCount,
			LastWithdrawal:    summary.LastWithdrawal,
			AverageWithdrawal: summary.AverageWithdrawal,
		}

		if summary.LastWithdrawalDate != nil {
			response.LastWithdrawalDate = summary.LastWithdrawalDate.Format(time.RFC3339)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// GetWithdrawalsByOrder возвращает информацию о выводе по номеру заказа
// GET /api/user/withdrawals/{order_number}
func GetWithdrawalsByOrder(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Проверяем аутентификацию
		login := auth.GetUserLogin(r.Context())
		if login == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Получаем номер заказа из URL
		orderNumber := r.URL.Path[len("/api/user/withdrawals/"):]
		if orderNumber == "" {
			http.Error(w, "Order number is required", http.StatusBadRequest)
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

		// Получаем информацию о выводе
		withdrawal, err := store.GetWithdrawalByOrder(r.Context(), userID, orderNumber)
		if err != nil {
			if err == storage.ErrWithdrawalNotFound {
				logger.Logger.Debugw("Withdrawal not found",
					"user_id", userID,
					"order", orderNumber)
				http.Error(w, "Withdrawal not found", http.StatusNotFound)
				return
			}
			logger.Logger.Errorw("Failed to get withdrawal by order",
				"user_id", userID,
				"order", orderNumber,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Формируем ответ
		response := WithdrawalResponse{
			Order:       withdrawal.OrderNumber,
			Sum:         withdrawal.Sum,
			ProcessedAt: withdrawal.ProcessedAt.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// WithdrawalResponse представляет ответ с информацией о выводе
type WithdrawalResponse struct {
	Order       string  `json:"order"`
	Sum         float64 `json:"sum"`
	ProcessedAt string  `json:"processed_at"`
}

// WithdrawalsSummaryResponse представляет сводку по выводам
type WithdrawalsSummaryResponse struct {
	TotalWithdrawn     float64 `json:"total_withdrawn"`
	WithdrawalsCount   int64   `json:"withdrawals_count"`
	AverageWithdrawal  float64 `json:"average_withdrawal"`
	LastWithdrawal     float64 `json:"last_withdrawal,omitempty"`
	LastWithdrawalDate string  `json:"last_withdrawal_date,omitempty"`
}

// CancelWithdrawal отменяет вывод средств (для администраторов)
// DELETE /api/admin/withdrawals/{id}
func CancelWithdrawal(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Проверяем права администратора
		login := auth.GetUserLogin(r.Context())
		if login == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Проверка, является ли пользователь администратором
		isAdmin, err := store.IsAdmin(r.Context(), login)
		if err != nil || !isAdmin {
			logger.Logger.Warnw("Unauthorized admin access attempt",
				"login", login,
				"action", "cancel_withdrawal")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Получаем ID вывода из URL
		withdrawalID := r.URL.Path[len("/api/admin/withdrawals/"):]
		if withdrawalID == "" {
			http.Error(w, "Withdrawal ID is required", http.StatusBadRequest)
			return
		}

		// Отменяем вывод
		err = store.CancelWithdrawal(r.Context(), withdrawalID)
		if err != nil {
			if err == storage.ErrWithdrawalNotFound {
				http.Error(w, "Withdrawal not found", http.StatusNotFound)
				return
			}
			logger.Logger.Errorw("Failed to cancel withdrawal",
				"withdrawal_id", withdrawalID,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logger.Logger.Infow("Withdrawal cancelled by admin",
			"admin", login,
			"withdrawal_id", withdrawalID)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"cancelled"}`))
	}
}
