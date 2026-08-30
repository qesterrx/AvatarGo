// cmd/service/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
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
	"github.com/qesterrx/AvatarGo/internal/telemetry"

	"github.com/qesterrx/AvatarGo/internal/handlers"
)

func main() {

	module := "avatargo-server"
	version := "v0.0.1"

	//Инициализация логгера
	err := logger.InitLogger(nil, "INFO")
	if err != nil {
		logger.Log.Fatal("Error init logger %v", err)
	}
	logger.Log.Info("Init logger - ОК")

	ctx := context.Background()

	//Инициализация OpenTelemetry логгера
	otClose, err := telemetry.InitTelemetryProvider(ctx, module, version)
	if err != nil {
		logger.Log.Fatal("Error init otl %v", err)
	}
	defer otClose()
	logger.Log.Info("Init logger - ОК")
	slog.InfoContext(ctx, "Starting")

	// Загружаем конфигурацию
	cfg, err := config.Load()
	if err != nil {
		slog.ErrorContext(ctx, "Error load config: "+err.Error())
		logger.Log.Fatal("Error load config %v", err)
	}
	logger.Log.Info("Load config - ОК")

	// Инициализируем репозиторий
	pg, err := repository.NewPGClient(&cfg.Database)
	if err != nil {
		slog.ErrorContext(ctx, "Error init PGSQL client: "+err.Error())
		logger.Log.Fatal("Error init PGSQL client %v", err)
	}
	defer pg.Close()
	logger.Log.Info("Init PGSQL client - ОК")

	// Подключаемся к S3 клиенту
	minio, err := repository.NewS3Client(&cfg.S3)
	if err != nil {
		slog.ErrorContext(ctx, "Error init S3 client: "+err.Error())
		logger.Log.Fatal("Error init S3 client: %v", err)
	}
	logger.Log.Info("Init S3 client- ОК")

	// Подключаемся к RabbitMQ
	rbt, err := broker.NewRabbitMq(&cfg.Rabbit)
	if err != nil {
		slog.ErrorContext(ctx, "Error init RabbitMQ client: "+err.Error())
		logger.Log.Fatal("Error init RabbitMQ client: %v", err)
	}
	defer rbt.Close()
	logger.Log.Info("Init RabbitMQ client - ОК")

	// Инициализируем сервис
	srv, err := services.NewAvatarService(pg, minio, rbt)
	if err != nil {
		slog.ErrorContext(ctx, "Error init service: "+err.Error())
		logger.Log.Fatal("Error init service: %v", err)
	}
	logger.Log.Info("Init service - ОК")

	// Инициализируем хендлеры
	hndls, err := handlers.NewHandlers(srv)
	if err != nil {
		slog.ErrorContext(ctx, "Error init handlers: "+err.Error())
		logger.Log.Fatal("Error init handlers: %v", err)
	}
	logger.Log.Info("Init handlers - ОК")

	// Инициализируем HTTP роутер
	router := hndls.GetRouter(module)

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
	logger.Log.Info("Application started")
	slog.InfoContext(ctx, "Application started")

	// Ожидаем сигналы для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Ожидаем либо сигнал, либо ошибку сервера
	select {
	case <-sigChan:
		slog.InfoContext(ctx, "Stop signal received")
		logger.Log.Info("Stop signal received")
	case err, ok := <-errCh:
		if ok && err != nil {
			slog.ErrorContext(ctx, "The server terminated with an error: "+err.Error())
			logger.Log.Error("The server terminated with an error: %v", err)
		} else {
			slog.InfoContext(ctx, "The server terminated before receiving the signal")
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
		slog.ErrorContext(ctx, "The server was forcibly stopped: "+err.Error())
		logger.Log.Error("The server was forcibly stopped: %v", err)
	}

	//Дожидакемся остановки сервера
	wg.Wait()

	slog.InfoContext(ctx, "Application stopped")
	logger.Log.Info("Application stopped")
}
