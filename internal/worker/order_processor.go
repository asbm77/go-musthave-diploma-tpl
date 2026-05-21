// internal/worker/order_processor.go
package worker

import (
	"context"
	"sync"
	"time"
)

type OrderRequest struct {
	OrderNumber string
	UserID      string
}

type OrderProcessor struct {
	storage       storage.Storage
	accrualClient *AccrualClient
	requestChan   chan OrderRequest
	bufferSize    int
	workers       int
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewOrderProcessor(storage storage.Storage, accrualAddr string, bufferSize, workers int) *OrderProcessor {
	return &OrderProcessor{
		storage:       storage,
		accrualClient: NewAccrualClient(accrualAddr),
		requestChan:   make(chan OrderRequest, bufferSize),
		bufferSize:    bufferSize,
		workers:       workers,
	}
}

func (op *OrderProcessor) Start() {
	op.ctx, op.cancel = context.WithCancel(context.Background())

	// Запускаем воркеров для параллельной обработки
	for i := 0; i < op.workers; i++ {
		op.wg.Add(1)
		go op.worker(i)
	}

	// Запускаем фоновую задачу для загрузки заказов из БД
	op.wg.Add(1)
	go op.loader()

	logger.Logger.Infow("Order processor started", "workers", op.workers, "buffer_size", op.bufferSize)
}

func (op *OrderProcessor) Stop() {
	op.cancel()
	op.wg.Wait()
	close(op.requestChan)
	logger.Logger.Info("Order processor stopped")
}

// GetQueue возвращает канал для отправки заказов на обработку
func (op *OrderProcessor) GetQueue() chan<- OrderRequest {
	return op.requestChan
}

// loader загружает заказы из БД и отправляет в канал
func (op *OrderProcessor) loader() {
	defer op.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-op.ctx.Done():
			return
		case <-ticker.C:
			// Загружаем заказы из БД
			orders, err := op.storage.GetPendingOrders(op.ctx, 100)
			if err != nil {
				logger.Logger.Errorw("Failed to load pending orders", "error", err)
				continue
			}

			// Отправляем в канал для обработки
			for _, order := range orders {
				select {
				case op.requestChan <- OrderRequest{
					OrderNumber: order.Number,
					UserID:      order.UserID,
				}:
				case <-op.ctx.Done():
					return
				}
			}
		}
	}
}

// worker обрабатывает заказы из канала
func (op *OrderProcessor) worker(id int) {
	defer op.wg.Done()

	logger.Logger.Infow("Worker started", "worker_id", id)

	for {
		select {
		case <-op.ctx.Done():
			logger.Logger.Infow("Worker stopping", "worker_id", id)
			return
		case req, ok := <-op.requestChan:
			if !ok {
				return
			}
			op.processOrder(req)
		}
	}
}

func (op *OrderProcessor) processOrder(req OrderRequest) {
	ctx, cancel := context.WithTimeout(op.ctx, 30*time.Second)
	defer cancel()

	// Обновляем статус на PROCESSING
	err := op.storage.UpdateOrderStatus(ctx, req.OrderNumber, storage.OrderStatusProcessing, nil)
	if err != nil {
		logger.Logger.Errorw("Failed to update order status",
			"order", req.OrderNumber,
			"error", err)
		return
	}

	// Запрашиваем информацию из системы расчёта
	resp, err := op.accrualClient.GetAccrual(ctx, req.OrderNumber)
	if err != nil {
		// При ошибке оставляем статус PROCESSING для повторной попытки
		logger.Logger.Errorw("Failed to get accrual info",
			"order", req.OrderNumber,
			"error", err)
		return
	}

	// Обновляем статус и начисление
	var accrual *float64
	if resp.Accrual > 0 {
		accrual = &resp.Accrual
	}

	status := mapAccrualStatus(resp.Status)
	err = op.storage.UpdateOrderStatus(ctx, req.OrderNumber, status, accrual)
	if err != nil {
		logger.Logger.Errorw("Failed to update order status with accrual",
			"order", req.OrderNumber,
			"status", status,
			"error", err)
	}
}

func mapAccrualStatus(accrualStatus string) string {
	switch accrualStatus {
	case "REGISTERED":
		return storage.OrderStatusNew
	case "PROCESSING":
		return storage.OrderStatusProcessing
	case "INVALID":
		return storage.OrderStatusInvalid
	case "PROCESSED":
		return storage.OrderStatusProcessed
	default:
		return storage.OrderStatusNew
	}
}
