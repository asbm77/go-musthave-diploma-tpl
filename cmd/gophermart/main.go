// cmd/server/main.go
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

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

	// Также читаем из окружения
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

	// Инициализируем логгер
	if err := logger.Initialize("info"); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	// Подключаемся к БД
	pgStorage, err := storage.NewPostgresStorage(flagDatabaseURI)
	if err != nil {
		logger.Logger.Fatalw("Failed to connect to database", "error", err)
	}
	defer pgStorage.Close()

	// Выполняем миграции
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := pgStorage.RunMigrations(ctx); err != nil {
		logger.Logger.Fatalw("Failed to run migrations", "error", err)
	}
	cancel()

	// Создаём воркер для обработки заказов (fan-in паттерн)
	orderProcessor := worker.NewOrderProcessor(pgStorage, flagAccrualAddr, 1000, 10)
	orderProcessor.Start()
	defer orderProcessor.Stop()

	// Инициализируем аутентификацию
	jwtAuth := auth.NewJWTAuth("your-secret-key-change-in-production")
	authHandler := handlers.NewAuthHandler(pgStorage, jwtAuth)

	// Создаём роутер
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(logger.HTTPLogger)

	// Публичные маршруты
	r.Post("/api/user/register", authHandler.Register)
	r.Post("/api/user/login", authHandler.Login)

	// Защищённые маршруты
	r.Group(func(r chi.Router) {
		r.Use(jwtAuth.Middleware)

		// Заказы
		r.Post("/api/user/orders", handlers.UploadOrder(pgStorage, orderProcessor))
		r.Get("/api/user/orders", handlers.GetUserOrders(pgStorage))

		// Баланс
		r.Get("/api/user/balance", handlers.GetBalance(pgStorage))
		r.Post("/api/user/balance/withdraw", handlers.Withdraw(pgStorage))

		// Выводы
		r.Get("/api/user/withdrawals", handlers.GetWithdrawals(pgStorage))
	})

	// Запускаем сервер
	srv := &http.Server{
		Addr:    flagRunAddr,
		Handler: r,
	}

	// Graceful shutdown
	go func() {
		logger.Logger.Infow("Starting server", "address", flagRunAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Fatalw("Server failed", "error", err)
		}
	}()

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
