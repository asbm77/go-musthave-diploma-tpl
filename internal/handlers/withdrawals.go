package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/storage"
	"github.com/asbm77/go-musthave-diploma-tpl/pkg/logger"
)

// WithdrawHandler обрабатывает запросы списания средств
type WithdrawHandler struct {
	storage *storage.PostgresStorage
}

// NewWithdrawHandler создаёт новый WithdrawHandler
func NewWithdrawHandler(storage *storage.PostgresStorage) *WithdrawHandler {
	return &WithdrawHandler{
		storage: storage,
	}
}

// Withdraw обрабатывает запрос на списание средств
func (h *WithdrawHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.GetUserIDFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req models.WithdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.Order == "" || req.Sum <= 0 {
		http.Error(w, "Invalid request: order and positive sum are required", http.StatusBadRequest)
		return
	}

	if !isValidLuhn(req.Order) {
		http.Error(w, "Invalid order number format", http.StatusUnprocessableEntity)
		return
	}

	withdrawal := &models.Withdrawal{
		UserID:      userID,
		OrderNumber: req.Order,
		Sum:         req.Sum,
		ProcessedAt: time.Now(),
	}

	if err := h.storage.WithdrawBalance(r.Context(), withdrawal); err != nil {
		if errors.Is(err, storage.ErrInsufficientFunds) {
			http.Error(w, "Insufficient funds", http.StatusPaymentRequired)
			return
		}
		logger.Logger.Errorw("Failed to withdraw balance", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// GetWithdrawals возвращает историю списаний пользователя
func (h *WithdrawHandler) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.GetUserIDFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	withdrawals, err := h.storage.GetUserWithdrawals(r.Context(), userID)
	if err != nil {
		logger.Logger.Errorw("Failed to get user withdrawals", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	response := make([]models.WithdrawalResponse, len(withdrawals))
	for i, wd := range withdrawals {
		response[i] = models.WithdrawalResponse{
			Order:       wd.OrderNumber,
			Sum:         wd.Sum,
			ProcessedAt: wd.ProcessedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
