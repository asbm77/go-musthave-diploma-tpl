package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/storage"
	"github.com/asbm77/go-musthave-diploma-tpl/pkg/logger"
)

// BalanceHandler обрабатывает запросы баланса
type BalanceHandler struct {
	storage *storage.PostgresStorage
}

// NewBalanceHandler создаёт новый BalanceHandler
func NewBalanceHandler(storage *storage.PostgresStorage) *BalanceHandler {
	return &BalanceHandler{
		storage: storage,
	}
}

// GetBalance возвращает текущий баланс пользователя
func (h *BalanceHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.GetUserIDFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	balance, err := h.storage.GetUserBalance(r.Context(), userID)
	if err != nil {
		logger.Logger.Errorw("Failed to get user balance", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := models.BalanceResponse{
		Current:   balance.Current,
		Withdrawn: balance.Withdrawn,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
