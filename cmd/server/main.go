// cmd/service/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/qesterrx/AvatarGo/internal/broker"
	"github.com/qesterrx/AvatarGo/internal/config"
	"github.com/qesterrx/AvatarGo/internal/repository"
	"github.com/qesterrx/AvatarGo/internal/services"
	"github.com/qesterrx/AvatarGo/internal/telemetry"

	"github.com/qesterrx/AvatarGo/internal/handlers"
)

func main() {

	module := "avatargo-server"
	version := "v0.0.1"

	ctx := context.Background()

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
	srv, err := services.NewAvatarService(pg, minio, rbt)
	if err != nil {
		slog.ErrorContext(ctx, "Error init service: "+err.Error())
		log.Fatal(fmt.Printf("Error init service: %v", err))
	}

	// Инициализируем хендлеры
	hndls, err := handlers.NewHandlers(srv)
	if err != nil {
		slog.ErrorContext(ctx, "Error init handlers: "+err.Error())
		log.Fatal(fmt.Printf("Error init handlers: %v", err))
	}

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
	slog.InfoContext(ctx, "Application started")

	// Ожидаем сигналы для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Ожидаем либо сигнал, либо ошибку сервера
	select {
	case <-sigChan:
		slog.InfoContext(ctx, "Stop signal received")
	case err, ok := <-errCh:
		if ok && err != nil {
			slog.ErrorContext(ctx, "The server terminated with an error: "+err.Error())
		} else {
			slog.InfoContext(ctx, "The server terminated before receiving the signal")
		}
		// Выходим, так как сервер уже не работает
		return
	}

	// Graceful shutdown с таймаутом
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()

	if err := server.Shutdown(ctxShutdown); err != nil {
		slog.ErrorContext(ctx, "The server was forcibly stopped: "+err.Error())
	}

	//Дожидакемся остановки сервера
	wg.Wait()

	slog.InfoContext(ctx, "Application stopped")
}
