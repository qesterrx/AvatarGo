// Package logger предоставляет настраиваемый логгер на основе библиотеки github.com/rs/zerolog
// Обеспечивает структурированное логирование с поддержкой уровней DEBUG, INFO и ERROR

package logger

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger представляет обёртку над zerolog.Logger, предоставляющую упрощённый интерфейс для логирования с поддержкой форматирования сообщений
type Logger struct {
	log zerolog.Logger
}

// Log - Переменная содержащая ссылку на логгер, доступный из любого места проекта
// По умолчанию инициализируется с zerolog.New(io.Discard) (ничего не выводит) до вызова InitLogger
var Log Logger = Logger{log: zerolog.New(io.Discard)}

// InitLogger выполняет первоначальную настройку глобального логгера (переменной Log).
// Устанавливает уровень логирования, выходной поток и форматирование
func InitLogger(io io.Writer, level string) error {

	if io == nil {
		io = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	}

	log := zerolog.New(io).With().Timestamp().Logger()
	switch {
	case level == "DEBUG":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case level == "INFO":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case level == "ERROR":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		return fmt.Errorf("неподдерживаемый уровень логирования. Передано %s, допустимые значения [DEBUG/INFO/ERROR]", level)
	}

	Log = Logger{log: log}

	return nil
}

// Debug логирует сообщение на уровне DEBUG
func (l *Logger) Debug(format string, a ...any) {
	l.log.Debug().Msg(fmt.Sprintf(format, a...))
}

// Debug логирует сообщение на уровне INFO
func (l *Logger) Info(format string, a ...any) {
	l.log.Info().Msg(fmt.Sprintf(format, a...))
}

// Debug логирует сообщение на уровне ERROR
func (l *Logger) Error(format string, a ...any) {
	l.log.Error().Msg(fmt.Sprintf(format, a...))
}

// ForwardError логирует ошибку на уровне ERROR с выводом её сообщения
func (l *Logger) ForwardError(err error) {
	l.log.Error().Msg(err.Error())
}

func (l *Logger) Fatal(format string, a ...any) {
	l.log.Error().Msg(fmt.Sprintf(format, a...))
	os.Exit(1)
}
