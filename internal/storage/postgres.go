package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserExists        = errors.New("user already exists")
	ErrOrderNotFound     = errors.New("order not found")
	ErrInsufficientFunds = errors.New("insufficient funds")
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// PostgresStorage реализует хранение данных в PostgreSQL
type PostgresStorage struct {
	db *sql.DB
}

// NewPostgresStorage создаёт новое подключение к PostgreSQL
func NewPostgresStorage(uri string) (*PostgresStorage, error) {
	db, err := sql.Open("pgx", uri)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	return &PostgresStorage{db: db}, nil
}

// Close закрывает соединение с БД
func (s *PostgresStorage) Close() error {
	return s.db.Close()
}

// RunMigrations выполняет миграции БД
func (s *PostgresStorage) RunMigrations(ctx context.Context) error {
	driver, err := postgres.WithInstance(s.db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://migrations",
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migration: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// CreateUser создаёт нового пользователя
func (s *PostgresStorage) CreateUser(ctx context.Context, login, password string) (*models.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO users (login, password_hash)
		VALUES ($1, $2)
		RETURNING id, login, created_at`

	var user models.User
	err = tx.QueryRowContext(ctx, query, login, password).Scan(&user.ID, &user.Login, &user.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return nil, ErrUserExists
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Создаём запись баланса для пользователя
	balanceQuery := `
		INSERT INTO balances (user_id, current, withdrawn)
		VALUES ($1, 0, 0)`

	if _, err := tx.ExecContext(ctx, balanceQuery, user.ID); err != nil {
		return nil, fmt.Errorf("failed to create balance: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &user, nil
}

// GetUserByLogin получает пользователя по логину
func (s *PostgresStorage) GetUserByLogin(ctx context.Context, login string) (*models.User, error) {
	query := `
		SELECT id, login, password_hash, created_at
		FROM users
		WHERE login = $1`

	var user models.User
	err := s.db.QueryRowContext(ctx, query, login).Scan(&user.ID, &user.Login, &user.Password, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// CreateOrder создаёт новый заказ
func (s *PostgresStorage) CreateOrder(ctx context.Context, order *models.Order) error {
	query := `
		INSERT INTO orders (number, user_id, status, uploaded_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := s.db.ExecContext(ctx, query, order.Number, order.UserID, order.Status, order.UploadedAt, order.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return ErrUserExists
		}
		return fmt.Errorf("failed to create order: %w", err)
	}

	return nil
}

// GetOrderByNumber получает заказ по номеру
func (s *PostgresStorage) GetOrderByNumber(ctx context.Context, number string) (*models.Order, error) {
	query := `
		SELECT number, user_id, status, accrual, uploaded_at, updated_at
		FROM orders
		WHERE number = $1`

	var order models.Order
	err := s.db.QueryRowContext(ctx, query, number).Scan(
		&order.Number, &order.UserID, &order.Status, &order.Accrual, &order.UploadedAt, &order.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	return &order, nil
}

// GetUserOrders получает все заказы пользователя
func (s *PostgresStorage) GetUserOrders(ctx context.Context, userID int64) ([]*models.Order, error) {
	query := `
		SELECT number, user_id, status, accrual, uploaded_at, updated_at
		FROM orders
		WHERE user_id = $1
		ORDER BY uploaded_at DESC`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user orders: %w", err)
	}
	defer rows.Close()

	var orders []*models.Order
	for rows.Next() {
		var order models.Order
		err := rows.Scan(&order.Number, &order.UserID, &order.Status, &order.Accrual, &order.UploadedAt, &order.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan order: %w", err)
		}
		orders = append(orders, &order)
	}

	return orders, nil
}

// UpdateOrderStatus обновляет статус заказа и начисление
func (s *PostgresStorage) UpdateOrderStatus(ctx context.Context, number string, status string, accrual *float64) error {
	query := `
		UPDATE orders
		SET status = $1, accrual = $2, updated_at = NOW()
		WHERE number = $3`

	_, err := s.db.ExecContext(ctx, query, status, accrual, number)
	if err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	// Если заказ обработан и есть начисление, обновляем баланс пользователя
	if status == "PROCESSED" && accrual != nil && *accrual > 0 {
		if err := s.updateUserBalance(ctx, number, *accrual); err != nil {
			return fmt.Errorf("failed to update user balance: %w", err)
		}
	}

	return nil
}

// updateUserBalance обновляет баланс пользователя при начислении баллов
func (s *PostgresStorage) updateUserBalance(ctx context.Context, orderNumber string, accrual float64) error {
	// Получаем user_id заказа
	var userID int64
	err := s.db.QueryRowContext(ctx, "SELECT user_id FROM orders WHERE number = $1", orderNumber).Scan(&userID)
	if err != nil {
		return fmt.Errorf("failed to get user_id: %w", err)
	}

	query := `
		UPDATE balances
		SET current = current + $1, updated_at = NOW()
		WHERE user_id = $2`

	_, err = s.db.ExecContext(ctx, query, accrual, userID)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	return nil
}

// GetUserBalance получает баланс пользователя
func (s *PostgresStorage) GetUserBalance(ctx context.Context, userID int64) (*models.Balance, error) {
	query := `
		SELECT user_id, current, withdrawn, updated_at
		FROM balances
		WHERE user_id = $1`

	var balance models.Balance
	err := s.db.QueryRowContext(ctx, query, userID).Scan(&balance.UserID, &balance.Current, &balance.Withdrawn, &balance.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &models.Balance{UserID: userID, Current: 0, Withdrawn: 0}, nil
		}
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	return &balance, nil
}

// WithdrawBalance списывает средства со счёта пользователя
func (s *PostgresStorage) WithdrawBalance(ctx context.Context, withdrawal *models.Withdrawal) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Проверяем и обновляем баланс
	updateQuery := `
		UPDATE balances
		SET current = current - $1,
		    withdrawn = withdrawn + $1,
		    updated_at = NOW()
		WHERE user_id = $2 AND current >= $1
		RETURNING current`

	var newCurrent float64
	err = tx.QueryRowContext(ctx, updateQuery, withdrawal.Sum, withdrawal.UserID).Scan(&newCurrent)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInsufficientFunds
		}
		return fmt.Errorf("failed to update balance: %w", err)
	}

	// Создаём запись о списании
	insertQuery := `
		INSERT INTO withdrawals (user_id, order_number, sum, processed_at)
		VALUES ($1, $2, $3, $4)`

	_, err = tx.ExecContext(ctx, insertQuery, withdrawal.UserID, withdrawal.OrderNumber, withdrawal.Sum, withdrawal.ProcessedAt)
	if err != nil {
		return fmt.Errorf("failed to create withdrawal record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetUserWithdrawals получает все списания пользователя
func (s *PostgresStorage) GetUserWithdrawals(ctx context.Context, userID int64) ([]*models.Withdrawal, error) {
	query := `
		SELECT id, user_id, order_number, sum, processed_at
		FROM withdrawals
		WHERE user_id = $1
		ORDER BY processed_at DESC`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get withdrawals: %w", err)
	}
	defer rows.Close()

	var withdrawals []*models.Withdrawal
	for rows.Next() {
		var wd models.Withdrawal
		err := rows.Scan(&wd.ID, &wd.UserID, &wd.OrderNumber, &wd.Sum, &wd.ProcessedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan withdrawal: %w", err)
		}
		withdrawals = append(withdrawals, &wd)
	}

	return withdrawals, nil
}

// GetPendingOrders получает заказы со статусом NEW или PROCESSING
func (s *PostgresStorage) GetPendingOrders(ctx context.Context) ([]*models.Order, error) {
	query := `
		SELECT number, user_id, status, accrual, uploaded_at, updated_at
		FROM orders
		WHERE status IN ('NEW', 'PROCESSING')
		ORDER BY uploaded_at ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending orders: %w", err)
	}
	defer rows.Close()

	var orders []*models.Order
	for rows.Next() {
		var order models.Order
		err := rows.Scan(&order.Number, &order.UserID, &order.Status, &order.Accrual, &order.UploadedAt, &order.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan order: %w", err)
		}
		orders = append(orders, &order)
	}

	return orders, nil
}
