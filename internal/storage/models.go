package storage

import (
	"time"
)

type User struct {
	ID        string    `db:"id"`
	Login     string    `db:"login"`
	Password  string    `db:"password"` // хэшированный
	CreatedAt time.Time `db:"created_at"`
}

type Order struct {
	Number     string    `db:"number"`
	UserID     string    `db:"user_id"`
	Status     string    `db:"status"`  // NEW, PROCESSING, INVALID, PROCESSED
	Accrual    *float64  `db:"accrual"` // может быть nil
	UploadedAt time.Time `db:"uploaded_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

type Balance struct {
	UserID    string    `db:"user_id"`
	Current   float64   `db:"current"`
	Withdrawn float64   `db:"withdrawn"`
	UpdatedAt time.Time `db:"updated_at"`
}

type Withdrawal struct {
	ID          string    `db:"id"`
	UserID      string    `db:"user_id"`
	OrderNumber string    `db:"order_number"`
	Sum         float64   `db:"sum"`
	ProcessedAt time.Time `db:"processed_at"`
}

// Статусы заказов
const (
	OrderStatusNew        = "NEW"
	OrderStatusProcessing = "PROCESSING"
	OrderStatusInvalid    = "INVALID"
	OrderStatusProcessed  = "PROCESSED"
)

// Ответ от системы расчёта
type AccrualResponse struct {
	Order   string  `json:"order"`
	Status  string  `json:"status"`
	Accrual float64 `json:"accrual,omitempty"`
}
