package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

func InitLoggerProvider(ctx context.Context, res *resource.Resource, module string) (func(), error) {

	// Создаём gRPC Exporter для логов (порт 4317)
	exporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP log exporter: %v", err)
	}

	// Инициализируем LoggerProvider
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)

	// Создаем slog Handler через otelslog bridge
	handler := otelslog.NewHandler(module, otelslog.WithLoggerProvider(loggerProvider))

	// Создаем slog логгер с этим handler'ом
	logger := slog.New(handler)

	// Устанавливаем как глобальный логгер
	slog.SetDefault(logger)

	// Возвращаем функцию для корректного завершения (flush данных перед выходом)
	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := loggerProvider.Shutdown(ctx); err != nil {
			otel.Handle(err)
		}
	}

	return shutdown, nil
}
