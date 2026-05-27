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
	flag.StringVar(&flagDatabaseURI, "d", "", "database URI")
	flag.StringVar(&flagAccrualAddr, "r", "", "accrual system address")
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

	// JWT секрет из переменной окружения
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		logger.Logger.Warn("JWT_SECRET not set, using default (not safe for production)")
		jwtSecret = "default-secret-change-in-production"
	}

	// Подключение к БД
	if flagDatabaseURI == "" {
		logger.Logger.Fatal("DATABASE_URI is required")
	}

	pgStorage, err := storage.NewPostgresStorage(flagDatabaseURI)
	if err != nil {
		logger.Logger.Fatalw("Failed to connect to database", "error", err)
	}
	defer pgStorage.Close()

	// Миграции
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := pgStorage.RunMigrations(ctx); err != nil {
		logger.Logger.Fatalw("Failed to run migrations", "error", err)
	}
	cancel()

	// Воркер для обработки заказов
	if flagAccrualAddr == "" {
		logger.Logger.Warn("ACCRUAL_SYSTEM_ADDRESS not set, order processing will be disabled")
	}
	orderProcessor := worker.NewOrderProcessor(pgStorage, flagAccrualAddr, 1000, 10)
	orderProcessor.Start()
	defer orderProcessor.Stop()

	// Аутентификация
	jwtAuth := auth.NewJWTAuth(jwtSecret)
	authHandler := handlers.NewAuthHandler(pgStorage, jwtAuth)

	// Роутер
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

		r.Post("/api/user/orders", handlers.UploadOrder(pgStorage, orderProcessor))
		r.Get("/api/user/orders", handlers.GetUserOrders(pgStorage))
		r.Get("/api/user/balance", handlers.GetBalance(pgStorage))
		r.Post("/api/user/balance/withdraw", handlers.Withdraw(pgStorage))
		r.Get("/api/user/withdrawals", handlers.GetWithdrawals(pgStorage))
	})

	// Сервер
	srv := &http.Server{
		Addr:    flagRunAddr,
		Handler: r,
	}

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
	srv.Shutdown(ctx)
	logger.Logger.Info("Server stopped")
}
