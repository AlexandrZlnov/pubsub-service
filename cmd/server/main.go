package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	//"time"

	"github.com/rs/zerolog"
	//"github.com/rs/zerolog/log"

	"github.com/AlexandrZlnov/pubsub-service/config"
	"github.com/AlexandrZlnov/pubsub-service/internal/server"
	"github.com/AlexandrZlnov/pubsub-service/internal/service"
	"github.com/AlexandrZlnov/pubsub-service/subpub"
)

func main() {
	// Инициализация логгера
	logger := zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger().
		Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Загрузка конфигурации
	cfg := config.NewDefaultConfig()

	// Инициализация шины событий
	bus := subpub.NewSubPub()
	defer bus.Close(context.Background())

	// Создание сервиса
	svc := service.NewPubSubService(bus, logger, cfg)
	defer svc.Close()

	// Создание gRPC сервера
	srv := server.NewServer(svc, logger, cfg)

	// Канал для обработки сигналов
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Запуск сервера в горутине
	go func() {
		if err := srv.Run(); err != nil {
			logger.Fatal().Err(err).Msg("failed to run server")
		}
	}()

	logger.Info().Msg("server started")

	// Ожидание сигнала завершения
	<-done
	logger.Info().Msg("server stopped")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(
		context.Background(),
		cfg.ShutdownTimeout,
	)
	defer cancel()

	if err := srv.GracefulStop(ctx); err != nil {
		logger.Error().Err(err).Msg("failed to stop server gracefully")
	}
}
