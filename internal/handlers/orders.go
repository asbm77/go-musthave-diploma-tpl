package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/storage"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/worker"
	"github.com/asbm77/go-musthave-diploma-tpl/pkg/logger"
)

// OrderHandler обрабатывает запросы заказов
type OrderHandler struct {
	storage   *storage.PostgresStorage
	processor *worker.OrderProcessor
}

// NewOrderHandler создаёт новый OrderHandler
func NewOrderHandler(storage *storage.PostgresStorage, processor *worker.OrderProcessor) *OrderHandler {
	return &OrderHandler{
		storage:   storage,
		processor: processor,
	}
}

// UploadOrder обрабатывает загрузку номера заказа
func (h *OrderHandler) UploadOrder(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.GetUserIDFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	orderNumber := strings.TrimSpace(string(body))
	if orderNumber == "" {
		http.Error(w, "Order number is required", http.StatusBadRequest)
		return
	}

	if !isValidLuhn(orderNumber) {
		http.Error(w, "Invalid order number format", http.StatusUnprocessableEntity)
		return
	}

	existingOrder, err := h.storage.GetOrderByNumber(r.Context(), orderNumber)
	if err != nil && !errors.Is(err, storage.ErrOrderNotFound) {
		logger.Logger.Errorw("Failed to check existing order", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if existingOrder != nil {
		if existingOrder.UserID == userID {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "Order already uploaded by another user", http.StatusConflict)
		return
	}

	order := &models.Order{
		Number:     orderNumber,
		UserID:     userID,
		Status:     "NEW",
		UploadedAt: time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := h.storage.CreateOrder(r.Context(), order); err != nil {
		logger.Logger.Errorw("Failed to create order", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.processor.ProcessOrder(order)
	w.WriteHeader(http.StatusAccepted)
}

// GetUserOrders возвращает список заказов пользователя
func (h *OrderHandler) GetUserOrders(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.GetUserIDFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	orders, err := h.storage.GetUserOrders(r.Context(), userID)
	if err != nil {
		logger.Logger.Errorw("Failed to get user orders", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	response := make([]models.OrderResponse, len(orders))
	for i, order := range orders {
		response[i] = models.OrderResponse{
			Number:     order.Number,
			Status:     order.Status,
			Accrual:    order.Accrual,
			UploadedAt: order.UploadedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func isValidLuhn(number string) bool {
	var sum int
	var alternate bool

	number = strings.ReplaceAll(number, " ", "")

	for i := len(number) - 1; i >= 0; i-- {
		n, err := strconv.Atoi(string(number[i]))
		if err != nil {
			return false
		}

		if alternate {
			n *= 2
			if n > 9 {
				n = n%10 + 1
			}
		}

		sum += n
		alternate = !alternate
	}

	return sum%10 == 0
}
