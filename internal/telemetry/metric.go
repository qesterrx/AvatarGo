package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

func InitMeterProvider(ctx context.Context, res *resource.Resource) (func(), error) {
	// Создаём OTel Exporter
	exporter, err := otlpmetricgrpc.New(
		ctx,
		otlpmetricgrpc.WithInsecure(),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metric exporter: %v", err)
	}

	// Инициализируем MeterProvider
	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(
			metric.NewPeriodicReader(
				exporter,
				metric.WithInterval(2*time.Second),
			),
		),
	)

	otel.SetMeterProvider(meterProvider)

	shutdown := func() {
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		if err := meterProvider.Shutdown(ctx); err != nil {
			otel.Handle(err)
		}
	}

	return shutdown, nil
}
