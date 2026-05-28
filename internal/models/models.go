package models

import "time"

// User представляет пользователя системы
type User struct {
	ID        int64     `json:"id"`
	Login     string    `json:"login"`
	Password  string    `json:"-"` // Пароль не возвращается в JSON
	CreatedAt time.Time `json:"created_at"`
}

// Order представляет заказ пользователя
type Order struct {
	Number     string    `json:"number"`
	UserID     int64     `json:"user_id"`
	Status     string    `json:"status"`
	Accrual    *float64  `json:"accrual,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Balance представляет баланс пользователя
type Balance struct {
	UserID    int64     `json:"user_id"`
	Current   float64   `json:"current"`
	Withdrawn float64   `json:"withdrawn"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Withdrawal представляет операцию списания
type Withdrawal struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	OrderNumber string    `json:"order"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at"`
}

// RegisterRequest запрос на регистрацию
type RegisterRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// LoginRequest запрос на аутентификацию
type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// WithdrawRequest запрос на списание
type WithdrawRequest struct {
	Order string  `json:"order"`
	Sum   float64 `json:"sum"`
}

// OrderResponse ответ с информацией о заказе
type OrderResponse struct {
	Number     string    `json:"number"`
	Status     string    `json:"status"`
	Accrual    *float64  `json:"accrual,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// BalanceResponse ответ с информацией о балансе
type BalanceResponse struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

// WithdrawalResponse ответ с информацией о списании
type WithdrawalResponse struct {
	Order       string    `json:"order"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at"`
}

// AccrualOrderResponse ответ от системы расчёта баллов
type AccrualOrderResponse struct {
	Order   string  `json:"order"`
	Status  string  `json:"status"`
	Accrual float64 `json:"accrual,omitempty"`
}
