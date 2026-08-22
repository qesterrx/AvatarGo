// cmd/service/main.go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/qesterrx/AvatarGo/internal/broker"
	"github.com/qesterrx/AvatarGo/internal/config"
	"github.com/qesterrx/AvatarGo/internal/logger"
	"github.com/qesterrx/AvatarGo/internal/repository"
	"github.com/qesterrx/AvatarGo/internal/worker"
	"golang.org/x/sync/errgroup"
)

func main() {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//Инициализация логгера
	err := logger.InitLogger(nil, "INFO")
	if err != nil {
		logger.Log.Fatal("Error init logger %v", err)
	}
	logger.Log.Info("Init logger - ОК")

	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		logger.Log.Fatal("Error load config %v", err)
	}
	logger.Log.Info("Load config - ОК")

	// Инициализируем репозиторий
	pg, err := repository.NewPGClient(&cfg.Database)
	if err != nil {
		logger.Log.Fatal("Error init PGSQL client %v", err)
	}
	defer pg.Close()
	logger.Log.Info("Init PGSQL client - ОК")

	// Подключаемся к S3 клиенту
	minio, err := repository.NewS3Client(&cfg.S3)
	if err != nil {
		logger.Log.Fatal("Error init S3 client: %v", err)
	}
	logger.Log.Info("Init S3 client- ОК")

	// Подключаемся к RabbitMQ
	rbt, err := broker.NewRabbitMq(&cfg.Rabbit)
	if err != nil {
		logger.Log.Fatal("Error init RabbitMQ client: %v", err)
	}
	defer rbt.Close()
	logger.Log.Info("Init RabbitMQ client - ОК")

	// Инициализируем сервис
	wrk, err := worker.NewAvatarWorker(pg, minio, rbt)
	if err != nil {
		logger.Log.Fatal("Error init workers: %v", err)
	}
	logger.Log.Info("Init workers - ОК")

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		logger.Log.Info("Running Deleting")
		return wrk.Deleting(ctx)
	})

	g.Go(func() error {
		logger.Log.Info("Running Uploading")
		return wrk.Uploading(ctx)
	})

	// Ожидаем сигналы для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Ждем сигнал завершения
	select {
	case <-sigChan:
		logger.Log.Info("Application stop signal received")
		cancel()
	case <-ctx.Done():
		logger.Log.Info("Emergency application stop")
	}

	g.Wait()

	logger.Log.Info("Application stopped")

}
