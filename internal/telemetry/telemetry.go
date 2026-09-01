package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func InitTelemetryProvider(ctx context.Context, module, version string) (func(), error) {

	// Метаинформация (Resource)
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithAttributes(
			semconv.ServiceVersionKey.String(version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %v", err)
	}

	LogClose, err := InitLoggerProvider(ctx, res, module)
	if err != nil {
		return nil, err
	}

	MetrClose, err := InitMeterProvider(ctx, res)
	if err != nil {
		return nil, err
	}

	TraceClose, err := InitTracerProvider(ctx, res)
	if err != nil {
		return nil, err
	}

	TelemetryProviderClose := func() {
		LogClose()
		MetrClose()
		TraceClose()
	}

	return TelemetryProviderClose, nil
}
