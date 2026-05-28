package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/handlers"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/middleware"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/storage"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/worker"
	"github.com/asbm77/go-musthave-diploma-tpl/pkg/logger"
)

var (
	flagRunAddr     string
	flagDatabaseURI string
	flagAccrualAddr string
)

func parseFlags() {
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "address and port to run server")
	flag.StringVar(&flagDatabaseURI, "d", "postgres://postgres:password@localhost:5432/loyalty?sslmode=disable", "database URI")
	flag.StringVar(&flagAccrualAddr, "r", "http://localhost:8081", "accrual system address")
	flag.Parse()

	if envAddr := os.Getenv("RUN_ADDRESS"); envAddr != "" {
		flagRunAddr = envAddr
	}
	if envDB := os.Getenv("DATABASE_URI"); envDB != "" {
		flagDatabaseURI = envDB
	}
	if envAccrual := os.Getenv("ACCRUAL_SYSTEM_ADDRESS"); envAccrual != "" {
		flagAccrualAddr = envAccrual
	}
}

func main() {
	parseFlags()

	// Инициализация логгера
	if err := logger.Initialize("info"); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Подключение к БД
	pgStorage, err := storage.NewPostgresStorage(flagDatabaseURI)
	if err != nil {
		logger.Logger.Fatalw("Failed to connect to database", "error", err)
	}
	defer pgStorage.Close()

	// Выполнение миграций
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := pgStorage.RunMigrations(ctx); err != nil {
		logger.Logger.Fatalw("Failed to run migrations", "error", err)
	}
	cancel()

	// Создание воркера для обработки заказов
	orderProcessor := worker.NewOrderProcessor(pgStorage, flagAccrualAddr, 100, 5)
	orderProcessor.Start()
	defer orderProcessor.Stop()

	// Инициализация JWT аутентификации
	jwtAuth := auth.NewJWTAuth("your-secret-key-change-in-production")

	// Создание хендлеров
	authHandler := handlers.NewAuthHandler(pgStorage, jwtAuth)
	orderHandler := handlers.NewOrderHandler(pgStorage, orderProcessor)
	balanceHandler := handlers.NewBalanceHandler(pgStorage)
	withdrawHandler := handlers.NewWithdrawHandler(pgStorage)

	// Создание маршрутизатора
	mux := http.NewServeMux()

	// Публичные маршруты
	mux.HandleFunc("POST /api/user/register", authHandler.Register)
	mux.HandleFunc("POST /api/user/login", authHandler.Login)

	// Защищённые маршруты
	mux.HandleFunc("POST /api/user/orders", middleware.RequireAuth(jwtAuth, orderHandler.UploadOrder))
	mux.HandleFunc("GET /api/user/orders", middleware.RequireAuth(jwtAuth, orderHandler.GetUserOrders))
	mux.HandleFunc("GET /api/user/balance", middleware.RequireAuth(jwtAuth, balanceHandler.GetBalance))
	mux.HandleFunc("POST /api/user/balance/withdraw", middleware.RequireAuth(jwtAuth, withdrawHandler.Withdraw))
	mux.HandleFunc("GET /api/user/withdrawals", middleware.RequireAuth(jwtAuth, withdrawHandler.GetWithdrawals))

	// Оборачивание в middleware
	var handler http.Handler = mux
	handler = middleware.RecoveryMiddleware(handler)
	handler = middleware.LoggingMiddleware(handler)

	// Запуск сервера
	srv := &http.Server{
		Addr:         flagRunAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Logger.Infow("Starting server", "address", flagRunAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Fatalw("Server failed", "error", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Logger.Info("Shutting down server...")

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Logger.Errorw("Server shutdown error", "error", err)
	}

	logger.Logger.Info("Server stopped")
}
