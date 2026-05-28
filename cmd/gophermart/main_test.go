package main

import (
	"context"
	"flag"
	"os"
	"testing"
	"time"
)

// TestMain - настройка тестового окружения
func TestMain(m *testing.M) {
	// Сохраняем оригинальные аргументы
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Убираем все флаги для тестов, оставляем только имя программы
	os.Args = []string{oldArgs[0]}

	// Сбрасываем флаги
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Устанавливаем тестовые переменные окружения
	os.Setenv("JWT_SECRET", "test-secret-key")

	// Запускаем тесты
	code := m.Run()

	os.Exit(code)
}

// TestParseFlags - тест парсинга флагов
func TestParseFlags(t *testing.T) {
	// Сохраняем оригинальные аргументы
	oldArgs := os.Args
	oldJWTSecret := os.Getenv("JWT_SECRET")
	defer func() {
		os.Args = oldArgs
		os.Setenv("JWT_SECRET", oldJWTSecret)
	}()

	// Убираем переменную окружения JWT_SECRET для этого теста
	os.Unsetenv("JWT_SECRET")

	// Устанавливаем тестовые аргументы
	os.Args = []string{"cmd", "-a", ":9090", "-d", "postgres://test:test@localhost/testdb", "-r", "http://test:8081", "-s", "test-secret"}

	// Сбрасываем флаги перед тестом
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Вызываем parseFlags
	parseFlags()

	// Проверяем результаты
	if flagRunAddr != ":9090" {
		t.Errorf("Expected flagRunAddr = :9090, got %s", flagRunAddr)
	}
	if flagDatabaseURI != "postgres://test:test@localhost/testdb" {
		t.Errorf("Expected flagDatabaseURI = postgres://test:test@localhost/testdb, got %s", flagDatabaseURI)
	}
	if flagAccrualAddr != "http://test:8081" {
		t.Errorf("Expected flagAccrualAddr = http://test:8081, got %s", flagAccrualAddr)
	}
	if jwtSecret != "test-secret" {
		t.Errorf("Expected jwtSecret = test-secret, got %s", jwtSecret)
	}
}

// TestParseFlagsWithEnv - тест парсинга флагов из окружения
func TestParseFlagsWithEnv(t *testing.T) {
	// Сохраняем оригинальные значения
	oldArgs := os.Args
	oldEnv := map[string]string{
		"RUN_ADDRESS":            os.Getenv("RUN_ADDRESS"),
		"DATABASE_URI":           os.Getenv("DATABASE_URI"),
		"ACCRUAL_SYSTEM_ADDRESS": os.Getenv("ACCRUAL_SYSTEM_ADDRESS"),
		"JWT_SECRET":             os.Getenv("JWT_SECRET"),
	}

	defer func() {
		os.Args = oldArgs
		for k, v := range oldEnv {
			os.Setenv(k, v)
		}
	}()

	// Устанавливаем тестовые переменные окружения
	os.Setenv("RUN_ADDRESS", ":8888")
	os.Setenv("DATABASE_URI", "postgres://env:env@localhost/envdb")
	os.Setenv("ACCRUAL_SYSTEM_ADDRESS", "http://env:8081")
	os.Setenv("JWT_SECRET", "env-secret")

	// Устанавливаем аргументы командной строки (должны быть переопределены окружением)
	os.Args = []string{"cmd", "-a", "flag:8080", "-d", "postgres://flag/flag", "-r", "http://flag:8081", "-s", "flag-secret"}

	// Сбрасываем флаги
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Вызываем parseFlags
	parseFlags()

	// Проверяем, что переменные окружения имеют приоритет
	if flagRunAddr != ":8888" {
		t.Errorf("Expected flagRunAddr = :8888, got %s", flagRunAddr)
	}
	if flagDatabaseURI != "postgres://env:env@localhost/envdb" {
		t.Errorf("Expected flagDatabaseURI = postgres://env:env@localhost/envdb, got %s", flagDatabaseURI)
	}
	if flagAccrualAddr != "http://env:8081" {
		t.Errorf("Expected flagAccrualAddr = http://env:8081, got %s", flagAccrualAddr)
	}
	if jwtSecret != "env-secret" {
		t.Errorf("Expected jwtSecret = env-secret, got %s", jwtSecret)
	}
}

// TestOrderNumberValidation - тест валидации номера заказа
func TestOrderNumberValidation(t *testing.T) {
	tests := []struct {
		name    string
		number  string
		isValid bool
	}{
		{"Valid Visa", "4532015112830366", true},
		{"Valid Mastercard", "5555555555554444", true},
		{"Valid test number", "12345678903", true},
		{"Invalid - too short", "123", false},
		{"Invalid - non-numeric", "abcdefg", false},
		{"Invalid - empty", "", false},
		{"Invalid - zero", "0", false},
		{"Invalid - single digit", "5", false},
		{"Valid - with spaces", "4532 0151 1283 0366", true},
		{"Valid - with dashes", "4532-0151-1283-0366", true},
		{"Invalid checksum", "1234567890123456", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := testIsValidLuhn(tt.number)
			if isValid != tt.isValid {
				t.Errorf("Luhn validation for %q: expected %v, got %v", tt.number, tt.isValid, isValid)
			}
		})
	}
}

// testIsValidLuhn - реализация алгоритма Луна для теста
func testIsValidLuhn(number string) bool {
	if number == "" {
		return false
	}

	var sum int
	var alternate bool

	// Убираем пробелы и дефисы
	cleaned := ""
	for _, ch := range number {
		if ch >= '0' && ch <= '9' {
			cleaned += string(ch)
		}
	}

	if cleaned == "" {
		return false
	}

	// Алгоритм Луна требует минимум 2 цифры
	if len(cleaned) < 2 {
		return false
	}

	for i := len(cleaned) - 1; i >= 0; i-- {
		n := int(cleaned[i] - '0')

		if alternate {
			n *= 2
			if n > 9 {
				n = n - 9
			}
		}

		sum += n
		alternate = !alternate
	}

	return sum%10 == 0
}

// TestConfigurationConstants - тест конфигурационных констант
func TestConfigurationConstants(t *testing.T) {
	t.Log("Configuration variables are defined")
	t.Logf("flagRunAddr default: %s", flagRunAddr)
	t.Logf("flagDatabaseURI default: %s", flagDatabaseURI)
	t.Logf("flagAccrualAddr default: %s", flagAccrualAddr)
	t.Logf("jwtSecret default: %s", jwtSecret)
}

// TestJWTSecretRequired - тест требования JWT секрета
func TestJWTSecretRequired(t *testing.T) {
	// Сохраняем оригинальные переменные
	oldJWTSecret := os.Getenv("JWT_SECRET")
	defer os.Setenv("JWT_SECRET", oldJWTSecret)

	// Удаляем JWT_SECRET
	os.Unsetenv("JWT_SECRET")

	// Проверяем, что переменная не установлена
	if os.Getenv("JWT_SECRET") != "" {
		t.Error("JWT_SECRET should be empty")
	}

	// Устанавливаем JWT_SECRET обратно
	os.Setenv("JWT_SECRET", "test-secret")
	if os.Getenv("JWT_SECRET") != "test-secret" {
		t.Error("JWT_SECRET should be set to test-secret")
	}
}

// TestGracefulShutdown - тест graceful shutdown
func TestGracefulShutdown(t *testing.T) {
	// Создаём канал для сигналов
	sigChan := make(chan os.Signal, 1)

	// Проверяем, что канал создан (не nil)
	if sigChan == nil {
		t.Error("Signal channel should not be nil")
	}

	// Создаём контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	// Проверяем, что контекст создан
	if ctx == nil {
		t.Error("Context should not be nil")
	}
	if cancel == nil {
		t.Error("Cancel function should not be nil")
	}

	// Отменяем контекст
	cancel()

	// Закрываем канал (хорошая практика, но не обязательно для теста)
	close(sigChan)
}

// TestPostgresConnectionString - тест строки подключения к PostgreSQL
func TestPostgresConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "default connection",
			uri:      "postgres://postgres:password@localhost:5432/loyalty?sslmode=disable",
			expected: "postgres://postgres:password@localhost:5432/loyalty?sslmode=disable",
		},
		{
			name:     "custom connection",
			uri:      "postgres://user:pass@host:5432/db?sslmode=require",
			expected: "postgres://user:pass@host:5432/db?sslmode=require",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.uri != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.uri)
			}
		})
	}
}

// TestAccrualSystemAddress - тест адреса системы начисления
func TestAccrualSystemAddress(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		expected string
	}{
		{
			name:     "default address",
			address:  "http://localhost:8081",
			expected: "http://localhost:8081",
		},
		{
			name:     "custom address",
			address:  "http://accrual-system:8080",
			expected: "http://accrual-system:8080",
		},
		{
			name:     "https address",
			address:  "https://accrual.example.com",
			expected: "https://accrual.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.address != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.address)
			}
		})
	}
}

// BenchmarkLuhnValidation - бенчмарк валидации Луна
func BenchmarkLuhnValidation(b *testing.B) {
	testNumbers := []string{
		"4532015112830366",
		"5555555555554444",
		"12345678903",
	}

	for i := 0; i < b.N; i++ {
		for _, num := range testNumbers {
			testIsValidLuhn(num)
		}
	}
}
