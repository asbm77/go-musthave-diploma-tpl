// internal/worker/pool.go
package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Task представляет единицу работы
type Task interface {
	Execute(ctx context.Context) error
	GetID() string
	GetPriority() int
}

// TaskFunc адаптер для функции как Task
type TaskFunc struct {
	ID       string
	Priority int
	Fn       func(ctx context.Context) error
}

func (t *TaskFunc) Execute(ctx context.Context) error {
	return t.Fn(ctx)
}

func (t *TaskFunc) GetID() string {
	return t.ID
}

func (t *TaskFunc) GetPriority() int {
	return t.Priority
}

// WorkerPool управляет пулом воркеров с fan-in очередью
type WorkerPool struct {
	name           string
	workerCount    int
	taskQueue      chan Task
	bufferSize     int
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	stats          *PoolStats
	mu             sync.RWMutex
	priorityQueues []chan Task // Приоритетные очереди (fan-in)
}

// PoolStats статистика работы пула
type PoolStats struct {
	TasksSubmitted uint64
	TasksCompleted uint64
	TasksFailed    uint64
	TasksQueued    uint64
	ActiveWorkers  int32
	LastTaskTime   time.Time
	QueueWaitTime  time.Duration
	ProcessingTime time.Duration
}

// PoolConfig конфигурация пула
type PoolConfig struct {
	Name         string
	WorkerCount  int
	BufferSize   int
	MaxQueueSize int
}

// DefaultPoolConfig возвращает конфигурацию по умолчанию
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		Name:         "default-pool",
		WorkerCount:  10,
		BufferSize:   1000,
		MaxQueueSize: 10000,
	}
}

// NewWorkerPool создаёт новый пул воркеров
func NewWorkerPool(config *PoolConfig) *WorkerPool {
	if config.WorkerCount <= 0 {
		config.WorkerCount = 10
	}
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}

	// Создаём очереди с приоритетами (fan-in)
	priorityQueues := make([]chan Task, 5) // 5 уровней приоритета
	for i := 0; i < 5; i++ {
		priorityQueues[i] = make(chan Task, config.BufferSize/5)
	}

	return &WorkerPool{
		name:           config.Name,
		workerCount:    config.WorkerCount,
		taskQueue:      make(chan Task, config.BufferSize),
		bufferSize:     config.BufferSize,
		priorityQueues: priorityQueues,
		stats:          &PoolStats{},
	}
}

// Start запускает пул воркеров
func (p *WorkerPool) Start() {
	p.ctx, p.cancel = context.WithCancel(context.Background())

	// Запускаем fan-in мультиплексор
	p.wg.Add(1)
	go p.fanInMultiplexer()

	// Запускаем воркеров
	for i := 0; i < p.workerCount; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	logger.Logger.Infow("Worker pool started",
		"name", p.name,
		"workers", p.workerCount,
		"buffer_size", p.bufferSize)
}

// Stop останавливает пул воркеров
func (p *WorkerPool) Stop() {
	logger.Logger.Infow("Stopping worker pool", "name", p.name)

	p.cancel()
	p.wg.Wait()
	close(p.taskQueue)

	for i := 0; i < len(p.priorityQueues); i++ {
		close(p.priorityQueues[i])
	}

	logger.Logger.Infow("Worker pool stopped",
		"name", p.name,
		"submitted", atomic.LoadUint64(&p.stats.TasksSubmitted),
		"completed", atomic.LoadUint64(&p.stats.TasksCompleted),
		"failed", atomic.LoadUint64(&p.stats.TasksFailed))
}

// fanInMultiplexer объединяет несколько очередей в одну (fan-in паттерн)
func (p *WorkerPool) fanInMultiplexer() {
	defer p.wg.Done()

	// Создаём таймер для периодического опроса
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			// Обрабатываем оставшиеся задачи перед выходом
			p.drainQueues()
			return

		case <-ticker.C:
			// Периодически проверяем приоритетные очереди
			p.processPriorityQueues()

		default:
			// Неблокирующая проверка всех очередей
			if !p.processPriorityQueues() {
				// Если нет задач в приоритетных очередях, ждём
				select {
				case task, ok := <-p.taskQueue:
					if !ok {
						return
					}
					p.processTask(task)
				case <-p.ctx.Done():
					return
				}
			}
		}
	}
}

// processPriorityQueues обрабатывает приоритетные очереди
func (p *WorkerPool) processPriorityQueues() bool {
	// Сначала проверяем высокие приоритеты (0 - самый высокий)
	for priority := 0; priority < len(p.priorityQueues); priority++ {
		select {
		case task, ok := <-p.priorityQueues[priority]:
			if !ok {
				continue
			}
			p.processTask(task)
			return true
		default:
			continue
		}
	}
	return false
}

// drainQueues опустошает все очереди перед остановкой
func (p *WorkerPool) drainQueues() {
	// Сначала приоритетные очереди
	for i := 0; i < len(p.priorityQueues); i++ {
		for task := range p.priorityQueues[i] {
			p.processTask(task)
		}
	}

	// Затем основную очередь
	for task := range p.taskQueue {
		p.processTask(task)
	}
}

// processTask обрабатывает одну задачу (отправляет воркеру или выполняет синхронно)
func (p *WorkerPool) processTask(task Task) {
	atomic.AddUint64(&p.stats.TasksQueued, 1)

	select {
	case p.taskQueue <- task:
		atomic.AddUint64(&p.stats.TasksSubmitted, 1)
	case <-p.ctx.Done():
		logger.Logger.Warnw("Task dropped due to shutdown",
			"task_id", task.GetID(),
			"pool", p.name)
	}
}

// Submit отправляет задачу в пул (обычный приоритет)
func (p *WorkerPool) Submit(task Task) error {
	select {
	case p.taskQueue <- task:
		atomic.AddUint64(&p.stats.TasksSubmitted, 1)
		return nil
	case <-p.ctx.Done():
		return ErrPoolClosed
	}
}

// SubmitPriority отправляет задачу с указанным приоритетом
// priority: 0 - самый высокий, 4 - самый низкий
func (p *WorkerPool) SubmitPriority(task Task, priority int) error {
	if priority < 0 || priority >= len(p.priorityQueues) {
		priority = 2 // default priority
	}

	select {
	case p.priorityQueues[priority] <- task:
		atomic.AddUint64(&p.stats.TasksSubmitted, 1)
		return nil
	case <-p.ctx.Done():
		return ErrPoolClosed
	}
}

// SubmitFunc отправляет функцию как задачу
func (p *WorkerPool) SubmitFunc(id string, fn func(ctx context.Context) error) error {
	return p.Submit(&TaskFunc{
		ID:       id,
		Priority: 2,
		Fn:       fn,
	})
}

// worker выполняет задачи из очереди
func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()

	logger.Logger.Debugw("Worker started",
		"pool", p.name,
		"worker_id", id)

	for {
		select {
		case <-p.ctx.Done():
			logger.Logger.Debugw("Worker stopping",
				"pool", p.name,
				"worker_id", id)
			return

		case task, ok := <-p.taskQueue:
			if !ok {
				return
			}

			p.executeTask(task, id)
		}
	}
}

// executeTask выполняет задачу с отслеживанием времени
func (p *WorkerPool) executeTask(task Task, workerID int) {
	atomic.AddInt32(&p.stats.ActiveWorkers, 1)
	defer atomic.AddInt32(&p.stats.ActiveWorkers, -1)

	startTime := time.Now()

	// Создаём контекст с таймаутом для задачи
	taskCtx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
	defer cancel()

	// Выполняем задачу
	err := task.Execute(taskCtx)

	duration := time.Since(startTime)

	// Обновляем статистику
	p.mu.Lock()
	p.stats.LastTaskTime = time.Now()
	p.stats.ProcessingTime = duration
	p.mu.Unlock()

	if err != nil {
		atomic.AddUint64(&p.stats.TasksFailed, 1)
		logger.Logger.Errorw("Task failed",
			"pool", p.name,
			"worker_id", workerID,
			"task_id", task.GetID(),
			"duration_ms", duration.Milliseconds(),
			"error", err)
	} else {
		atomic.AddUint64(&p.stats.TasksCompleted, 1)
		logger.Logger.Debugw("Task completed",
			"pool", p.name,
			"worker_id", workerID,
			"task_id", task.GetID(),
			"duration_ms", duration.Milliseconds())
	}
}

// GetStats возвращает текущую статистику пула
func (p *WorkerPool) GetStats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return PoolStats{
		TasksSubmitted: atomic.LoadUint64(&p.stats.TasksSubmitted),
		TasksCompleted: atomic.LoadUint64(&p.stats.TasksCompleted),
		TasksFailed:    atomic.LoadUint64(&p.stats.TasksFailed),
		TasksQueued:    atomic.LoadUint64(&p.stats.TasksQueued),
		ActiveWorkers:  atomic.LoadInt32(&p.stats.ActiveWorkers),
		LastTaskTime:   p.stats.LastTaskTime,
		QueueWaitTime:  p.stats.QueueWaitTime,
		ProcessingTime: p.stats.ProcessingTime,
	}
}

// GetQueueLength возвращает текущую длину очереди
func (p *WorkerPool) GetQueueLength() int {
	return len(p.taskQueue)
}

// GetCapacity возвращает ёмкость буфера
func (p *WorkerPool) GetCapacity() int {
	return cap(p.taskQueue)
}

// IsFull проверяет, заполнен ли буфер
func (p *WorkerPool) IsFull() bool {
	return len(p.taskQueue) >= cap(p.taskQueue)
}

// GetUtilization возвращает процент заполнения буфера
func (p *WorkerPool) GetUtilization() float64 {
	if p.bufferSize == 0 {
		return 0
	}
	return float64(len(p.taskQueue)) / float64(p.bufferSize) * 100
}

// WaitForCompletion ожидает завершения всех задач
func (p *WorkerPool) WaitForCompletion(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats := p.GetStats()
			if stats.TasksSubmitted == stats.TasksCompleted+stats.TasksFailed {
				return nil
			}
			if time.Now().After(deadline) {
				return ErrTimeout
			}
		case <-p.ctx.Done():
			return ErrPoolClosed
		}
	}
}

// Ошибки
var (
	ErrPoolClosed = &PoolError{Message: "pool is closed"}
	ErrTimeout    = &PoolError{Message: "timeout waiting for completion"}
)

// PoolError ошибка пула
type PoolError struct {
	Message string
}

func (e *PoolError) Error() string {
	return e.Message
}

// BatchProcessor для пакетной обработки с fan-in
type BatchProcessor struct {
	pool         *WorkerPool
	batchSize    int
	flushTimeout time.Duration
	buffer       []Task
	mu           sync.Mutex
	flushTicker  *time.Ticker
	stopCh       chan struct{}
}

// NewBatchProcessor создаёт новый пакетный процессор
func NewBatchProcessor(pool *WorkerPool, batchSize int, flushTimeout time.Duration) *BatchProcessor {
	bp := &BatchProcessor{
		pool:         pool,
		batchSize:    batchSize,
		flushTimeout: flushTimeout,
		buffer:       make([]Task, 0, batchSize),
		flushTicker:  time.NewTicker(flushTimeout),
		stopCh:       make(chan struct{}),
	}

	go bp.run()
	return bp
}

// run запускает фоновую обработку для периодического сброса буфера
func (bp *BatchProcessor) run() {
	for {
		select {
		case <-bp.flushTicker.C:
			bp.Flush()
		case <-bp.stopCh:
			bp.flushTicker.Stop()
			bp.Flush()
			return
		}
	}
}

// Add добавляет задачу в буфер
func (bp *BatchProcessor) Add(task Task) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.buffer = append(bp.buffer, task)

	if len(bp.buffer) >= bp.batchSize {
		return bp.flush()
	}

	return nil
}

// Flush принудительно сбрасывает буфер
func (bp *BatchProcessor) Flush() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return bp.flush()
}

// flush внутренний метод для сброса буфера
func (bp *BatchProcessor) flush() error {
	if len(bp.buffer) == 0 {
		return nil
	}

	// Копируем буфер
	batch := make([]Task, len(bp.buffer))
	copy(batch, bp.buffer)
	bp.buffer = bp.buffer[:0]

	// Создаём пакетную задачу
	batchTask := &BatchTask{
		id:    generateBatchID(),
		tasks: batch,
	}

	// Отправляем на обработку
	return bp.pool.Submit(batchTask)
}

// Stop останавливает пакетный процессор
func (bp *BatchProcessor) Stop() {
	close(bp.stopCh)
}

// BatchTask пакетная задача
type BatchTask struct {
	id    string
	tasks []Task
}

func (bt *BatchTask) Execute(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(bt.tasks))

	for _, task := range bt.tasks {
		wg.Add(1)
		go func(t Task) {
			defer wg.Done()
			if err := t.Execute(ctx); err != nil {
				errCh <- err
			}
		}(task)
	}

	wg.Wait()
	close(errCh)

	// Собираем ошибки
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		logger.Logger.Warnw("Batch task completed with errors",
			"batch_id", bt.id,
			"total_tasks", len(bt.tasks),
			"errors_count", len(errors))
	}

	return nil
}

func (bt *BatchTask) GetID() string {
	return bt.id
}

func (bt *BatchTask) GetPriority() int {
	return 1 // Высокий приоритет для пакетов
}

func generateBatchID() string {
	return time.Now().Format("20060102150405") + "-batch"
}

// OrderTask задача для обработки заказа
type OrderTask struct {
	OrderNumber string
	UserID      string
	ExecuteFunc func(ctx context.Context, orderNumber, userID string) error
}

func (t *OrderTask) Execute(ctx context.Context) error {
	return t.ExecuteFunc(ctx, t.OrderNumber, t.UserID)
}

func (t *OrderTask) GetID() string {
	return "order-" + t.OrderNumber
}

func (t *OrderTask) GetPriority() int {
	return 1 // Высокий приоритет для заказов
}
