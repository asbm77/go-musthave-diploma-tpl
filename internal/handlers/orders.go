// internal/handlers/orders.go
package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/logger"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/storage"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/utils"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/worker"
)

// UploadOrder обрабатывает загрузку номера заказа
// POST /api/user/orders
func UploadOrder(store storage.Storage, processor *worker.OrderProcessor) http.HandlerFunc {
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
			logger.Logger.Errorw("Failed to get user ID", "login", login, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Читаем тело запроса (номер заказа в текстовом формате)
		body := make([]byte, 0, 512)
		n, err := r.Body.Read(body)
		if err != nil && n == 0 {
			// Читаем по-другому, так как Read может не прочитать всё
			var readErr error
			body, readErr = io.ReadAll(r.Body)
			if readErr != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}
		}
		defer r.Body.Close()

		orderNumber := strings.TrimSpace(string(body))
		if orderNumber == "" {
			http.Error(w, "Order number is required", http.StatusBadRequest)
			return
		}

		// Проверяем номер заказа по алгоритму Луна
		if !utils.ValidateLuhn(orderNumber) {
			logger.Logger.Debugw("Invalid order number format",
				"order", orderNumber,
				"user_id", userID)
			http.Error(w, "Invalid order number format", http.StatusUnprocessableEntity)
			return
		}

		// Пытаемся создать заказ
		err = store.CreateOrder(r.Context(), orderNumber, userID)
		if err != nil {
			switch err {
			case storage.ErrOrderExists:
				// Заказ уже загружен этим пользователем
				logger.Logger.Debugw("Order already exists for this user",
					"order", orderNumber,
					"user_id", userID)
				w.WriteHeader(http.StatusOK)
				return

			case storage.ErrOrderBelongsToAnotherUser:
				// Заказ принадлежит другому пользователю
				logger.Logger.Warnw("Order belongs to another user",
					"order", orderNumber,
					"user_id", userID)
				http.Error(w, "Order already uploaded by another user", http.StatusConflict)
				return

			default:
				logger.Logger.Errorw("Failed to create order",
					"order", orderNumber,
					"user_id", userID,
					"error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		// Отправляем заказ на обработку в воркер (fan-in)
		select {
		case processor.GetQueue() <- worker.OrderRequest{
			OrderNumber: orderNumber,
			UserID:      userID,
		}:
			logger.Logger.Infow("Order accepted for processing",
				"order", orderNumber,
				"user_id", userID)
			w.WriteHeader(http.StatusAccepted)

		default:
			// Если очередь переполнена, возвращаем ошибку
			logger.Logger.Warnw("Order queue is full",
				"order", orderNumber,
				"user_id", userID)
			http.Error(w, "Server busy, please try again later", http.StatusServiceUnavailable)
		}
	}
}

// GetUserOrders возвращает список заказов пользователя
// GET /api/user/orders
func GetUserOrders(store storage.Storage) http.HandlerFunc {
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
			logger.Logger.Errorw("Failed to get user ID", "login", login, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Получаем заказы пользователя
		orders, err := store.GetUserOrders(r.Context(), userID)
		if err != nil {
			logger.Logger.Errorw("Failed to get user orders",
				"user_id", userID,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Если заказов нет, возвращаем 204 No Content
		if len(orders) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Формируем ответ
		response := make([]OrderResponse, 0, len(orders))
		for _, order := range orders {
			resp := OrderResponse{
				Number:     order.Number,
				Status:     order.Status,
				UploadedAt: order.UploadedAt.Format(time.RFC3339),
			}

			// Добавляем начисление, если оно есть
			if order.Accrual != nil && *order.Accrual > 0 {
				resp.Accrual = order.Accrual
			}

			response = append(response, resp)
		}

		// Отправляем ответ
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Logger.Errorw("Failed to encode response", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logger.Logger.Debugw("User orders retrieved",
			"user_id", userID,
			"count", len(response))
	}
}

// OrderResponse представляет ответ при получении списка заказов
type OrderResponse struct {
	Number     string   `json:"number"`
	Status     string   `json:"status"`
	Accrual    *float64 `json:"accrual,omitempty"`
	UploadedAt string   `json:"uploaded_at"`
}

// GetOrderByNumber возвращает информацию о конкретном заказе (для внутреннего использования)
func GetOrderByNumber(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Извлекаем номер заказа из URL
		orderNumber := strings.TrimPrefix(r.URL.Path, "/api/orders/")
		if orderNumber == "" {
			http.Error(w, "Order number is required", http.StatusBadRequest)
			return
		}

		// Получаем заказ
		order, err := store.GetOrderByNumber(r.Context(), orderNumber)
		if err != nil {
			if err == storage.ErrOrderNotFound {
				http.Error(w, "Order not found", http.StatusNotFound)
				return
			}
			logger.Logger.Errorw("Failed to get order",
				"order", orderNumber,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Формируем ответ
		response := OrderResponse{
			Number:     order.Number,
			Status:     order.Status,
			UploadedAt: order.UploadedAt.Format(time.RFC3339),
		}

		if order.Accrual != nil && *order.Accrual > 0 {
			response.Accrual = order.Accrual
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Logger.Errorw("Failed to encode response", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

// ProcessOrdersBatch фоновая задача для пакетной обработки заказов
func ProcessOrdersBatch(store storage.Storage, processor *worker.OrderProcessor) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		// Получаем заказы, ожидающие обработки
		orders, err := store.GetPendingOrders(ctx, 100)
		cancel()

		if err != nil {
			logger.Logger.Errorw("Failed to get pending orders", "error", err)
			continue
		}

		if len(orders) == 0 {
			continue
		}

		logger.Logger.Infow("Processing pending orders batch",
			"count", len(orders))

		// Отправляем заказы в обработку
		for _, order := range orders {
			select {
			case processor.GetQueue() <- worker.OrderRequest{
				OrderNumber: order.Number,
				UserID:      order.UserID,
			}:
			default:
				logger.Logger.Warnw("Order queue is full, skipping",
					"order", order.Number)
			}
		}
	}
}

// OrderStatusUpdate обновляет статус заказа (вебхук от системы расчёта)
func OrderStatusUpdate(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Order   string  `json:"order"`
			Status  string  `json:"status"`
			Accrual float64 `json:"accrual,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		var accrual *float64
		if req.Accrual > 0 {
			accrual = &req.Accrual
		}

		// Обновляем статус заказа
		err := store.UpdateOrderStatus(r.Context(), req.Order, req.Status, accrual)
		if err != nil {
			logger.Logger.Errorw("Failed to update order status",
				"order", req.Order,
				"status", req.Status,
				"error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logger.Logger.Infow("Order status updated via webhook",
			"order", req.Order,
			"status", req.Status,
			"accrual", req.Accrual)

		w.WriteHeader(http.StatusOK)
	}
}

// GetOrderStats возвращает статистику по заказам (для администрирования)
func GetOrderStats(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := store.GetOrderStats(r.Context())
		if err != nil {
			logger.Logger.Errorw("Failed to get order stats", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(stats)
	}
}

// OrderStats статистика по заказам
type OrderStats struct {
	TotalOrders     int64            `json:"total_orders"`
	PendingOrders   int64            `json:"pending_orders"`
	ProcessedOrders int64            `json:"processed_orders"`
	InvalidOrders   int64            `json:"invalid_orders"`
	StatusCount     map[string]int64 `json:"status_count"`
	TotalAccrual    float64          `json:"total_accrual"`
}
