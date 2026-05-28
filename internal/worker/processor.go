package worker

import (
	"context"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/accrual"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/storage"
	"github.com/asbm77/go-musthave-diploma-tpl/pkg/logger"
)

// OrderProcessor обрабатывает заказы асинхронно
type OrderProcessor struct {
	storage       *storage.PostgresStorage
	accrualClient *accrual.Client
	ordersChan    chan *models.Order
	workersCount  int
	stopChan      chan struct{}
}

// NewOrderProcessor создаёт новый OrderProcessor
func NewOrderProcessor(storage *storage.PostgresStorage, accrualAddr string, bufferSize int, workersCount int) *OrderProcessor {
	return &OrderProcessor{
		storage:       storage,
		accrualClient: accrual.NewClient(accrualAddr),
		ordersChan:    make(chan *models.Order, bufferSize),
		workersCount:  workersCount,
		stopChan:      make(chan struct{}),
	}
}

// Start запускает воркеров
func (p *OrderProcessor) Start() {
	for i := 0; i < p.workersCount; i++ {
		go p.worker()
	}
	logger.Logger.Infow("Order processor started", "workers", p.workersCount)
}

// Stop останавливает воркеров
func (p *OrderProcessor) Stop() {
	close(p.stopChan)
	logger.Logger.Info("Order processor stopped")
}

// ProcessOrder отправляет заказ на обработку
func (p *OrderProcessor) ProcessOrder(order *models.Order) {
	select {
	case p.ordersChan <- order:
	default:
		logger.Logger.Warnw("Order channel full, dropping order", "order", order.Number)
	}
}

// worker обрабатывает заказы из очереди
func (p *OrderProcessor) worker() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case order := <-p.ordersChan:
			p.processOrderWithRetry(order)
		case <-ticker.C:
			// Периодически проверяем незавершённые заказы
			p.processPendingOrders()
		}
	}
}

// processOrderWithRetry обрабатывает заказ с повторными попытками
func (p *OrderProcessor) processOrderWithRetry(order *models.Order) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Используем клиент с повторными попытками
	info, err := p.accrualClient.GetOrderInfoWithRetry(ctx, order.Number, 5, 2*time.Second)
	if err != nil {
		logger.Logger.Errorw("Failed to process order after retries", "order", order.Number, "error", err)
		return
	}

	if info == nil {
		// Заказ не зарегистрирован в системе, пробуем позже
		logger.Logger.Debugw("Order not registered in accrual system", "order", order.Number)
		return
	}

	var accrual *float64
	if info.Accrual > 0 {
		accrual = &info.Accrual
	}

	if err := p.storage.UpdateOrderStatus(context.Background(), order.Number, info.Status, accrual); err != nil {
		logger.Logger.Errorw("Failed to update order status", "error", err, "order", order.Number)
		return
	}

	logger.Logger.Infow("Order status updated", "order", order.Number, "status", info.Status, "accrual", info.Accrual)
}

// processPendingOrders обрабатывает заказы, ожидающие обработки
func (p *OrderProcessor) processPendingOrders() {
	orders, err := p.storage.GetPendingOrders(context.Background())
	if err != nil {
		logger.Logger.Errorw("Failed to get pending orders", "error", err)
		return
	}

	for _, order := range orders {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		info, err := p.accrualClient.GetOrderInfo(ctx, order.Number)
		if err != nil {
			logger.Logger.Errorw("Failed to get order info from accrual system", "error", err, "order", order.Number)
			cancel()
			continue
		}

		if info != nil {
			var accrual *float64
			if info.Accrual > 0 {
				accrual = &info.Accrual
			}
			if err := p.storage.UpdateOrderStatus(context.Background(), order.Number, info.Status, accrual); err != nil {
				logger.Logger.Errorw("Failed to update order status", "error", err, "order", order.Number)
			}
		}

		cancel()
		time.Sleep(100 * time.Millisecond) // Небольшая задержка между запросами
	}
}

// GetAccrualClient возвращает клиент системы расчёта
func (p *OrderProcessor) GetAccrualClient() *accrual.Client {
	return p.accrualClient
}
