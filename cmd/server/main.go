// cmd/service/main.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/qesterrx/AvatarGo/internal/broker"
	"github.com/qesterrx/AvatarGo/internal/config"
	"github.com/qesterrx/AvatarGo/internal/logger"
	"github.com/qesterrx/AvatarGo/internal/repository"
	"github.com/qesterrx/AvatarGo/internal/services"

	"github.com/qesterrx/AvatarGo/internal/handlers"
)

func main() {

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
	srv, err := services.NewAvatarService(pg, minio, rbt)
	if err != nil {
		logger.Log.Fatal("Ошибка инициализации сервиса: %v", err)
	}
	logger.Log.Info("Инициализация Сервиса - ОК")

	// Инициализируем хендлеры
	hndls, err := handlers.NewHandlers(srv)
	if err != nil {
		logger.Log.Fatal("Ошибка инициализации обработчиков: %v", err)
	}
	logger.Log.Info("Инициализация Обработчиков - ОК")

	// Инициализируем HTTP роутер
	router := hndls.GetRouter()

	// Создаем HTTP сервер
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	var wg sync.WaitGroup
	wg.Add(1)

	errCh := make(chan error, 1)

	go func() {
		defer wg.Done()
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()
	logger.Log.Info("Запуск сервера ...")

	// Ожидаем сигналы для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Ожидаем либо сигнал, либо ошибку сервера
	select {
	case <-sigChan:
		logger.Log.Info("Получен сигнал остановки")
	case err, ok := <-errCh:
		if ok && err != nil {
			logger.Log.Error("Сервер завершился с ошибкой: %v", err)
		} else {
			logger.Log.Info("Сервер завершился до получения сигнала")
		}
		// Выходим, так как сервер уже не работает
		return
	}

	logger.Log.Info("Остановка сервера...")

	// Graceful shutdown с таймаутом
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()

	if err := server.Shutdown(ctxShutdown); err != nil {
		logger.Log.Error("Сервер остановлен принудительно: %v", err)
	}

	//Дожидакемся остановки сервера
	wg.Wait()

	logger.Log.Info("Работа завершена")
}
