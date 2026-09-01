// cmd/service/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/qesterrx/AvatarGo/internal/broker"
	"github.com/qesterrx/AvatarGo/internal/config"
	"github.com/qesterrx/AvatarGo/internal/repository"
	"github.com/qesterrx/AvatarGo/internal/telemetry"
	"github.com/qesterrx/AvatarGo/internal/worker"
	"golang.org/x/sync/errgroup"
)

func main() {

	module := "avatargo-worker"
	version := "v0.0.1"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//Инициализация OpenTelemetry логгера
	otClose, err := telemetry.InitTelemetryProvider(ctx, module, version)
	if err != nil {
		log.Fatal(fmt.Printf("Error init otl %v", err))
	}
	defer otClose()
	slog.InfoContext(ctx, "Starting")

	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		slog.ErrorContext(ctx, "Error load config: "+err.Error())
		log.Fatal(fmt.Printf("Error load config %v", err))
	}

	// Инициализируем репозиторий
	pg, err := repository.NewPGClient(&cfg.Database)
	if err != nil {
		slog.ErrorContext(ctx, "Error init PGSQL client: "+err.Error())
		log.Fatal(fmt.Printf("Error init PGSQL client %v", err))
	}
	defer pg.Close()

	// Подключаемся к S3 клиенту
	minio, err := repository.NewS3Client(&cfg.S3)
	if err != nil {
		slog.ErrorContext(ctx, "Error init S3 client: "+err.Error())
		log.Fatal(fmt.Printf("Error init S3 client: %v", err))
	}

	// Подключаемся к RabbitMQ
	rbt, err := broker.NewRabbitMq(&cfg.Rabbit)
	if err != nil {
		slog.ErrorContext(ctx, "Error init RabbitMQ client: "+err.Error())
		log.Fatal(fmt.Printf("Error init RabbitMQ client: %v", err))
	}
	defer rbt.Close()

	// Инициализируем сервис
	wrk, err := worker.NewAvatarWorker(pg, minio, rbt)
	if err != nil {
		slog.ErrorContext(ctx, "Error init RabbitMQ client: "+err.Error())
		log.Fatal(fmt.Printf("Error init RabbitMQ client: %v", err))
	}
	slog.InfoContext(ctx, "Init workers - ОК")

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		slog.InfoContext(ctx, "Running Deleting")
		return wrk.Deleting(ctx)
	})

	g.Go(func() error {
		slog.InfoContext(ctx, "Running Uploading")
		return wrk.Uploading(ctx)
	})

	slog.InfoContext(ctx, "Application started")

	// Ожидаем сигналы для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Ждем сигнал завершения
	select {
	case <-sigChan:
		slog.InfoContext(ctx, "Application stop signal received")
		cancel()
	case <-ctx.Done():
		slog.InfoContext(ctx, "Emergency application stop")
	}

	g.Wait()

	slog.InfoContext(ctx, "Application stopped")

}
