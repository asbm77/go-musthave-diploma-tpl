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
		login := auth.GetUserLogin(r.Context())
		if login == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		userID, err := store.GetUserIDByLogin(r.Context(), login)
		if err != nil {
			logger.Logger.Errorw("Failed to get user ID", "login", login, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		withdrawals, err := store.GetWithdrawals(r.Context(), userID)
		if err != nil {
			logger.Logger.Errorw("Failed to get withdrawals", "user_id", userID, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if len(withdrawals) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		response := make([]WithdrawalResponse, 0, len(withdrawals))
		for _, wd := range withdrawals {
			response = append(response, WithdrawalResponse{
				Order:       wd.OrderNumber,
				Sum:         wd.Sum,
				ProcessedAt: wd.ProcessedAt.Format(time.RFC3339),
			})
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
