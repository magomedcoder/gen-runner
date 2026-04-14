package main

import (
	"context"
	"fmt"
	"github.com/magomedcoder/gen/internal/config"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/magomedcoder/gen/internal/app/di"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc"
)

func main() {
	ctx := context.Background()
	if err := run(ctx, "./configs/config.yaml"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfgPath string) error {
	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		logger.Default.SetLevel(logger.LevelInfo)
		logger.E("Ошибка загрузки конфигурации: %v", err)
		return err
	}

	logger.Default.SetLevel(logger.ParseLevel(cfg.LogLevel))

	c, err := di.New(ctx, cfg)
	if err != nil {
		logger.E("Сборка приложения: %v", err)
		return err
	}

	defer func() {
		if err := c.Close(); err != nil {
			logger.W("Закрытие ресурсов: %v", err)
		}
	}()

	grpcServer := grpc.NewServer()
	c.RegisterGRPC(grpcServer)

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.E("Ошибка запуска сервера на адресе %s: %v", addr, err)
		return err
	}

	logger.I("Сервер запущен на %s", addr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- grpcServer.Serve(listener)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		grpcServer.GracefulStop()
		logger.I("Сервер остановлен")
		return nil
	case err := <-errCh:
		if err != nil {
			logger.E("Ошибка работы сервера: %v", err)
			return err
		}

		return nil
	}
}
