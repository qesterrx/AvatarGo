package logger

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestInitLogger(t *testing.T) {
	// Сохраняем оригинальный логгер
	oldLog := Log
	defer func() {
		Log = oldLog
	}()

	err := InitLogger(io.Discard, "INFO")
	assert.NoError(t, err)

	// Проверяем уровень логирования
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Errorf("Expected InfoLevel, got %v", zerolog.GlobalLevel())
	}
}

func TestLogMessages(t *testing.T) {
	// Сохраняем оригинальный логгер
	oldLog := Log
	defer func() {
		Log = oldLog
	}()

	// Создаем буфер для перехвата вывода
	var buf bytes.Buffer
	writer := zerolog.ConsoleWriter{Out: &buf, TimeFormat: time.RFC3339, NoColor: true}
	err := InitLogger(writer, "DEBUG")
	assert.NoError(t, err)

	tests := []struct {
		name     string
		logFunc  func()
		expected string
	}{
		{
			name: "Debug message",
			logFunc: func() {
				Log.Debug("test debug")
			},
			expected: "test debug",
		},
		{
			name: "Info message",
			logFunc: func() {
				Log.Info("test info")
			},
			expected: "test info",
		},
		{
			name: "Error message",
			logFunc: func() {
				Log.Error("test error")
			},
			expected: "test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc()

			if !strings.Contains(buf.String(), tt.expected) {
				t.Errorf("Expected log to contain '%s', got '%s'", tt.expected, buf.String())
			}
		})
	}
}
