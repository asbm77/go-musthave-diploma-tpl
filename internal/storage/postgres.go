// internal/storage/postgres.go
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type PostgresStorage struct {
	db *sql.DB
}

func NewPostgresStorage(dsn string) (*PostgresStorage, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)

	return &PostgresStorage{db: db}, nil
}

// RunMigrations создаёт необходимые таблицы
func (s *PostgresStorage) RunMigrations(ctx context.Context) error {
	queries := []string{
		// Таблица пользователей
		`CREATE TABLE IF NOT EXISTS users (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            login VARCHAR(255) UNIQUE NOT NULL,
            password VARCHAR(255) NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )`,

		// Таблица заказов
		`CREATE TABLE IF NOT EXISTS orders (
            number VARCHAR(255) PRIMARY KEY,
            user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            status VARCHAR(50) NOT NULL DEFAULT 'NEW',
            accrual DECIMAL(10,2),
            uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )`,

		// Индексы для orders
		`CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_uploaded_at ON orders(uploaded_at DESC)`,

		// Таблица баланса
		`CREATE TABLE IF NOT EXISTS balances (
            user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
            current DECIMAL(10,2) DEFAULT 0,
            withdrawn DECIMAL(10,2) DEFAULT 0,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )`,

		// Таблица выводов
		`CREATE TABLE IF NOT EXISTS withdrawals (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            order_number VARCHAR(255) NOT NULL,
            sum DECIMAL(10,2) NOT NULL,
            processed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )`,

		`CREATE INDEX IF NOT EXISTS idx_withdrawals_user_id ON withdrawals(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_withdrawals_processed_at ON withdrawals(processed_at DESC)`,

		// Триггер для обновления updated_at
		`CREATE OR REPLACE FUNCTION update_updated_at_column()
         RETURNS TRIGGER AS $$
         BEGIN
             NEW.updated_at = CURRENT_TIMESTAMP;
             RETURN NEW;
         END;
         $$ language 'plpgsql'`,

		`DROP TRIGGER IF EXISTS update_orders_updated_at ON orders`,
		`CREATE TRIGGER update_orders_updated_at
         BEFORE UPDATE ON orders
         FOR EACH ROW
         EXECUTE FUNCTION update_updated_at_column()`,
	}

	for _, query := range queries {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// CreateUser создаёт нового пользователя
func (s *PostgresStorage) CreateUser(ctx context.Context, login, hashedPassword string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var userID string
	query := `INSERT INTO users (login, password) VALUES ($1, $2) RETURNING id`
	err = tx.QueryRowContext(ctx, query, login, hashedPassword).Scan(&userID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrLoginExists
		}
		return err
	}

	// Создаём запись баланса
	_, err = tx.ExecContext(ctx, `INSERT INTO balances (user_id) VALUES ($1)`, userID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetUser возвращает пользователя по логину
func (s *PostgresStorage) GetUser(ctx context.Context, login string) (*User, error) {
	var user User
	query := `SELECT id, login, password FROM users WHERE login = $1`
	err := s.db.QueryRowContext(ctx, query, login).Scan(&user.ID, &user.Login, &user.Password)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	return &user, err
}

// CreateOrder создаёт новый заказ (блокировка через SELECT FOR UPDATE)
func (s *PostgresStorage) CreateOrder(ctx context.Context, orderNumber, userID string) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Проверяем, существует ли заказ
	var existingUserID string
	query := `SELECT user_id FROM orders WHERE number = $1 FOR UPDATE`
	err = tx.QueryRowContext(ctx, query, orderNumber).Scan(&existingUserID)

	if err == nil {
		if existingUserID == userID {
			return ErrOrderExists
		}
		return ErrOrderBelongsToAnotherUser
	}

	if err != sql.ErrNoRows {
		return err
	}

	// Создаём новый заказ
	insertQuery := `INSERT INTO orders (number, user_id, status) VALUES ($1, $2, 'NEW')`
	_, err = tx.ExecContext(ctx, insertQuery, orderNumber, userID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetUserOrders возвращает заказы пользователя
func (s *PostgresStorage) GetUserOrders(ctx context.Context, userID string) ([]Order, error) {
	query := `
        SELECT number, status, COALESCE(accrual, 0), uploaded_at
        FROM orders
        WHERE user_id = $1
        ORDER BY uploaded_at DESC
    `

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var order Order
		var accrual sql.NullFloat64
		err := rows.Scan(&order.Number, &order.Status, &accrual, &order.UploadedAt)
		if err != nil {
			return nil, err
		}
		if accrual.Valid {
			order.Accrual = &accrual.Float64
		}
		orders = append(orders, order)
	}

	return orders, rows.Err()
}

// UpdateOrderStatus обновляет статус и начисление заказа
func (s *PostgresStorage) UpdateOrderStatus(ctx context.Context, orderNumber, status string, accrual *float64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Обновляем заказ
	query := `UPDATE orders SET status = $1, accrual = $2 WHERE number = $3`
	_, err = tx.ExecContext(ctx, query, status, accrual, orderNumber)
	if err != nil {
		return err
	}

	// Если заказ обработан успешно, начисляем баллы
	if status == OrderStatusProcessed && accrual != nil && *accrual > 0 {
		// Получаем user_id заказа
		var userID string
		err = tx.QueryRowContext(ctx, `SELECT user_id FROM orders WHERE number = $1`, orderNumber).Scan(&userID)
		if err != nil {
			return err
		}

		// Обновляем баланс
		updateBalance := `
            UPDATE balances 
            SET current = current + $1, updated_at = CURRENT_TIMESTAMP
            WHERE user_id = $2
        `
		_, err = tx.ExecContext(ctx, updateBalance, *accrual, userID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetBalance возвращает баланс пользователя
func (s *PostgresStorage) GetBalance(ctx context.Context, userID string) (*Balance, error) {
	var balance Balance
	query := `SELECT current, withdrawn FROM balances WHERE user_id = $1`
	err := s.db.QueryRowContext(ctx, query, userID).Scan(&balance.Current, &balance.Withdrawn)
	if err == sql.ErrNoRows {
		return &Balance{Current: 0, Withdrawn: 0}, nil
	}
	return &balance, err
}

// Withdraw списывает средства
func (s *PostgresStorage) Withdraw(ctx context.Context, userID, orderNumber string, sum float64) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Проверяем баланс
	var current float64
	query := `SELECT current FROM balances WHERE user_id = $1 FOR UPDATE`
	err = tx.QueryRowContext(ctx, query, userID).Scan(&current)
	if err != nil {
		return err
	}

	if current < sum {
		return ErrInsufficientFunds
	}

	// Обновляем баланс
	updateBalance := `
        UPDATE balances 
        SET current = current - $1, withdrawn = withdrawn + $1, updated_at = CURRENT_TIMESTAMP
        WHERE user_id = $2
    `
	_, err = tx.ExecContext(ctx, updateBalance, sum, userID)
	if err != nil {
		return err
	}

	// Записываем транзакцию вывода
	insertWithdrawal := `
        INSERT INTO withdrawals (user_id, order_number, sum)
        VALUES ($1, $2, $3)
    `
	_, err = tx.ExecContext(ctx, insertWithdrawal, userID, orderNumber, sum)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetWithdrawals возвращает историю выводов
func (s *PostgresStorage) GetWithdrawals(ctx context.Context, userID string) ([]Withdrawal, error) {
	query := `
        SELECT order_number, sum, processed_at
        FROM withdrawals
        WHERE user_id = $1
        ORDER BY processed_at DESC
    `

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var withdrawals []Withdrawal
	for rows.Next() {
		var w Withdrawal
		err := rows.Scan(&w.OrderNumber, &w.Sum, &w.ProcessedAt)
		if err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, w)
	}

	return withdrawals, rows.Err()
}

// GetPendingOrders возвращает заказы, требующие проверки (для воркера)
func (s *PostgresStorage) GetPendingOrders(ctx context.Context, limit int) ([]Order, error) {
	query := `
        SELECT number, user_id
        FROM orders
        WHERE status IN ('NEW', 'PROCESSING')
        ORDER BY uploaded_at ASC
        LIMIT $1
        FOR UPDATE SKIP LOCKED
    `

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var order Order
		err := rows.Scan(&order.Number, &order.UserID)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

func isUniqueViolation(err error) bool {
	if pqErr, ok := err.(*pq.Error); ok {
		return pqErr.Code == "23505"
	}
	return false
}

// Ошибки
var (
	ErrLoginExists               = fmt.Errorf("login already exists")
	ErrUserNotFound              = fmt.Errorf("user not found")
	ErrOrderExists               = fmt.Errorf("order already exists for this user")
	ErrOrderBelongsToAnotherUser = fmt.Errorf("order belongs to another user")
	ErrInsufficientFunds         = fmt.Errorf("insufficient funds")
)
