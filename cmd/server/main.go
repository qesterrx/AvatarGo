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
	srv, err := services.NewAvatarService(pg, minio, rbt)
	if err != nil {
		logger.Log.Fatal("Error init service: %v", err)
	}
	logger.Log.Info("Init service - ОК")

	// Инициализируем хендлеры
	hndls, err := handlers.NewHandlers(srv)
	if err != nil {
		logger.Log.Fatal("Error init handlers: %v", err)
	}
	logger.Log.Info("Init handlers - ОК")

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
	logger.Log.Info("Running server ...")

	// Ожидаем сигналы для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Ожидаем либо сигнал, либо ошибку сервера
	select {
	case <-sigChan:
		logger.Log.Info("Stop signal received")
	case err, ok := <-errCh:
		if ok && err != nil {
			logger.Log.Error("The server terminated with an error: %v", err)
		} else {
			logger.Log.Info("The server terminated before receiving the signal")
		}
		// Выходим, так как сервер уже не работает
		return
	}

	logger.Log.Info("Server shutdown...")

	// Graceful shutdown с таймаутом
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()

	if err := server.Shutdown(ctxShutdown); err != nil {
		logger.Log.Error("The server was forcibly stopped: %v", err)
	}

	//Дожидакемся остановки сервера
	wg.Wait()

	logger.Log.Info("Application stopped")
}
