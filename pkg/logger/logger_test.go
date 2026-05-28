package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitialize(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		wantErr bool
	}{
		{
			name:    "debug level",
			level:   "debug",
			wantErr: false,
		},
		{
			name:    "info level",
			level:   "info",
			wantErr: false,
		},
		{
			name:    "warn level",
			level:   "warn",
			wantErr: false,
		},
		{
			name:    "error level",
			level:   "error",
			wantErr: false,
		},
		{
			name:    "invalid level",
			level:   "invalid",
			wantErr: false, // Should default to info
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Initialize(tt.level)
			assert.NoError(t, err)
			assert.NotNil(t, Logger)

			// Test logging functions
			Logger.Debug("debug message")
			Logger.Info("info message")
			Logger.Warn("warn message")
			Logger.Error("error message")

			// Test with structured logging
			Logger.Infow("structured log", "key", "value")
		})
	}
}

func TestSync(t *testing.T) {
	// Should not panic
	Sync()

	// Initialize and sync
	err := Initialize("info")
	assert.NoError(t, err)
	Sync()
}
