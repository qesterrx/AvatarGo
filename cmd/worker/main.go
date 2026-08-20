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
		logger.Log.Fatal("Ошибка инициализации логгера %v", err)
	}
	logger.Log.Info("Инициализация логгера - ОК")

	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		logger.Log.Fatal("Ошибка конфигурации %v", err)
	}
	logger.Log.Info("Загрузка конфигурации - ОК")

	// Инициализируем репозиторий
	pg, err := repository.NewPGClient(&cfg.Database)
	if err != nil {
		logger.Log.Fatal("Ошибка инициализации PGSQL клиента %v", err)
	}
	defer pg.Close()
	logger.Log.Info("Инициализация PGSQL - ОК")

	// Подключаемся к S3 клиенту
	minio, err := repository.NewS3Client(&cfg.S3)
	if err != nil {
		logger.Log.Fatal("Ошибка инициализации S3 клиента: %v", err)
	}
	logger.Log.Info("Инициализация S3 - ОК")

	// Подключаемся к RabbitMQ
	rbt, err := broker.NewRabbitMq(&cfg.Rabbit)
	if err != nil {
		logger.Log.Fatal("Ошибка инициализации RabbitMQ клиента: %v", err)
	}
	defer rbt.Close()
	logger.Log.Info("Инициализация RabbitMQ - ОК")

	// Инициализируем сервис
	wrk, err := worker.NewAvatarWorker(pg, minio, rbt)
	if err != nil {
		logger.Log.Fatal("Ошибка инициализации воркера: %v", err)
	}
	logger.Log.Info("Инициализация Воркера - ОК")

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		logger.Log.Info("Запуск Deleting")
		return wrk.Deleting(ctx)
	})

	g.Go(func() error {
		logger.Log.Info("Запуск Uploading")
		return wrk.Uploading(ctx)
	})

	// Ожидаем сигналы для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Ждем сигнал завершения
	select {
	case <-sigChan:
		logger.Log.Info("Получен сигнал остановки приложения")
		cancel()
	case <-ctx.Done():
		logger.Log.Info("Экстренная остановка приложения")
	}

	g.Wait()

	logger.Log.Info("Работа завершена")

}
