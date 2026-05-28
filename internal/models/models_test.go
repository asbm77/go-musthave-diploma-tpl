package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUserJSON(t *testing.T) {
	user := User{
		ID:        1,
		Login:     "testuser",
		Password:  "secret",
		CreatedAt: time.Now(),
	}

	// Marshal
	data, err := json.Marshal(user)
	assert.NoError(t, err)

	// Password should not be in JSON
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.Equal(t, float64(1), result["id"])
	assert.Equal(t, "testuser", result["login"])
	_, exists := result["password"]
	assert.False(t, exists)
}

func TestOrderResponse(t *testing.T) {
	accrual := float64(500)
	response := OrderResponse{
		Number:     "12345678903",
		Status:     "PROCESSED",
		Accrual:    &accrual,
		UploadedAt: time.Now(),
	}

	data, err := json.Marshal(response)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.Equal(t, "12345678903", result["number"])
	assert.Equal(t, "PROCESSED", result["status"])
	assert.Equal(t, 500.0, result["accrual"])
}

func TestBalanceResponse(t *testing.T) {
	response := BalanceResponse{
		Current:   1000.50,
		Withdrawn: 200.25,
	}

	data, err := json.Marshal(response)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.Equal(t, 1000.5, result["current"])
	assert.Equal(t, 200.25, result["withdrawn"])
}
