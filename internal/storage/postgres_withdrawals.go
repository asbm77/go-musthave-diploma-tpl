// internal/storage/postgres_withdrawals.go
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// WithdrawalSummary представляет сводку по выводам
type WithdrawalSummary struct {
	TotalWithdrawn     float64
	WithdrawalsCount   int64
	AverageWithdrawal  float64
	LastWithdrawal     float64
	LastWithdrawalDate *time.Time
}

// GetWithdrawalsSummary возвращает сводку по выводам пользователя
func (s *PostgresStorage) GetWithdrawalsSummary(ctx context.Context, userID string) (*WithdrawalSummary, error) {
	summary := &WithdrawalSummary{}

	query := `
		SELECT 
			COALESCE(SUM(sum), 0) as total_withdrawn,
			COUNT(*) as withdrawals_count,
			COALESCE(AVG(sum), 0) as average_withdrawal,
			(
				SELECT sum 
				FROM withdrawals 
				WHERE user_id = $1 
				ORDER BY processed_at DESC 
				LIMIT 1
			) as last_withdrawal,
			(
				SELECT processed_at 
				FROM withdrawals 
				WHERE user_id = $1 
				ORDER BY processed_at DESC 
				LIMIT 1
			) as last_withdrawal_date
		FROM withdrawals
		WHERE user_id = $1
	`

	var lastWithdrawal sql.NullFloat64
	var lastWithdrawalDate sql.NullTime

	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&summary.TotalWithdrawn,
		&summary.WithdrawalsCount,
		&summary.AverageWithdrawal,
		&lastWithdrawal,
		&lastWithdrawalDate,
	)

	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if lastWithdrawal.Valid {
		summary.LastWithdrawal = lastWithdrawal.Float64
	}

	if lastWithdrawalDate.Valid {
		summary.LastWithdrawalDate = &lastWithdrawalDate.Time
	}

	return summary, nil
}

// GetWithdrawalByOrder возвращает информацию о выводе по номеру заказа
func (s *PostgresStorage) GetWithdrawalByOrder(ctx context.Context, userID, orderNumber string) (*Withdrawal, error) {
	var withdrawal Withdrawal

	query := `
		SELECT id, user_id, order_number, sum, processed_at
		FROM withdrawals
		WHERE user_id = $1 AND order_number = $2
	`

	err := s.db.QueryRowContext(ctx, query, userID, orderNumber).Scan(
		&withdrawal.ID,
		&withdrawal.UserID,
		&withdrawal.OrderNumber,
		&withdrawal.Sum,
		&withdrawal.ProcessedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrWithdrawalNotFound
	}

	return &withdrawal, err
}

// CancelWithdrawal отменяет вывод средств и возвращает средства на счёт
func (s *PostgresStorage) CancelWithdrawal(ctx context.Context, withdrawalID string) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Получаем информацию о выводе
	var userID string
	var sum float64
	var orderNumber string

	getQuery := `
		SELECT user_id, sum, order_number
		FROM withdrawals
		WHERE id = $1
		FOR UPDATE
	`

	err = tx.QueryRowContext(ctx, getQuery, withdrawalID).Scan(&userID, &sum, &orderNumber)
	if err == sql.ErrNoRows {
		return ErrWithdrawalNotFound
	}
	if err != nil {
		return err
	}

	// Возвращаем средства на баланс
	updateBalance := `
		UPDATE balances
		SET current = current + $1, 
		    withdrawn = withdrawn - $1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $2
	`

	_, err = tx.ExecContext(ctx, updateBalance, sum, userID)
	if err != nil {
		return err
	}

	// Удаляем запись о выводе (или помечаем как отменённую)
	deleteQuery := `DELETE FROM withdrawals WHERE id = $1`
	_, err = tx.ExecContext(ctx, deleteQuery, withdrawalID)
	if err != nil {
		return err
	}

	// Записываем историю отмены
	historyQuery := `
		INSERT INTO balance_history (user_id, amount, type, description, order_number)
		VALUES ($1, $2, $3, $4, $5)
	`

	description := "Withdrawal cancelled: " + withdrawalID
	_, err = tx.ExecContext(ctx, historyQuery, userID, sum, "accrual", description, orderNumber)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// IsAdmin проверяет, является ли пользователь администратором
func (s *PostgresStorage) IsAdmin(ctx context.Context, login string) (bool, error) {
	var isAdmin bool
	query := `SELECT is_admin FROM users WHERE login = $1`
	err := s.db.QueryRowContext(ctx, query, login).Scan(&isAdmin)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return isAdmin, err
}

// Ошибки
var (
	ErrWithdrawalNotFound = fmt.Errorf("withdrawal not found")
)

// GetWithdrawalsByPeriod возвращает выводы за указанный период
func (s *PostgresStorage) GetWithdrawalsByPeriod(ctx context.Context, userID string, from, to time.Time) ([]Withdrawal, error) {
	query := `
		SELECT id, user_id, order_number, sum, processed_at
		FROM withdrawals
		WHERE user_id = $1 
		  AND processed_at >= $2 
		  AND processed_at <= $3
		ORDER BY processed_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var withdrawals []Withdrawal
	for rows.Next() {
		var w Withdrawal
		err := rows.Scan(&w.ID, &w.UserID, &w.OrderNumber, &w.Sum, &w.ProcessedAt)
		if err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, w)
	}

	return withdrawals, rows.Err()
}

// GetTotalWithdrawnToday возвращает сумму выводов за сегодня
func (s *PostgresStorage) GetTotalWithdrawnToday(ctx context.Context, userID string) (float64, error) {
	var total float64

	query := `
		SELECT COALESCE(SUM(sum), 0)
		FROM withdrawals
		WHERE user_id = $1 
		  AND processed_at >= CURRENT_DATE
	`

	err := s.db.QueryRowContext(ctx, query, userID).Scan(&total)
	return total, err
}

// GetWithdrawalsStats возвращает статистику по выводам для администрирования
func (s *PostgresStorage) GetWithdrawalsStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Общая статистика
	query := `
		SELECT 
			COUNT(*) as total_withdrawals,
			COALESCE(SUM(sum), 0) as total_amount,
			COUNT(DISTINCT user_id) as unique_users,
			AVG(sum) as average_amount,
			MIN(processed_at) as first_withdrawal,
			MAX(processed_at) as last_withdrawal
		FROM withdrawals
	`

	var totalWithdrawals int64
	var totalAmount float64
	var uniqueUsers int64
	var averageAmount float64
	var firstWithdrawal, lastWithdrawal sql.NullTime

	err := s.db.QueryRowContext(ctx, query).Scan(
		&totalWithdrawals,
		&totalAmount,
		&uniqueUsers,
		&averageAmount,
		&firstWithdrawal,
		&lastWithdrawal,
	)

	if err != nil {
		return nil, err
	}

	stats["total_withdrawals"] = totalWithdrawals
	stats["total_amount"] = totalAmount
	stats["unique_users"] = uniqueUsers
	stats["average_amount"] = averageAmount

	if firstWithdrawal.Valid {
		stats["first_withdrawal"] = firstWithdrawal.Time
	}
	if lastWithdrawal.Valid {
		stats["last_withdrawal"] = lastWithdrawal.Time
	}

	// Выводы по дням
	dailyQuery := `
		SELECT 
			DATE(processed_at) as date,
			COUNT(*) as count,
			COALESCE(SUM(sum), 0) as total
		FROM withdrawals
		WHERE processed_at >= CURRENT_DATE - INTERVAL '30 days'
		GROUP BY DATE(processed_at)
		ORDER BY date DESC
	`

	rows, err := s.db.QueryContext(ctx, dailyQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dailyStats []map[string]interface{}
	for rows.Next() {
		var date time.Time
		var count int64
		var total float64

		if err := rows.Scan(&date, &count, &total); err != nil {
			return nil, err
		}

		dailyStats = append(dailyStats, map[string]interface{}{
			"date":  date,
			"count": count,
			"total": total,
		})
	}

	stats["daily_stats"] = dailyStats

	return stats, nil
}
