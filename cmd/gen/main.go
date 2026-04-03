package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/magomedcoder/gen/config"
	"github.com/magomedcoder/gen/internal/app/di"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc"
)

func main() {
	configPath := flag.String("config", "config.yaml", "путь к YAML-конфигурации")
	flag.Parse()

	cfg, err := config.LoadFrom(*configPath)
	if err != nil {
		logger.Default.SetLevel(logger.LevelInfo)
		logger.E("Ошибка загрузки конфигурации: %v", err)
		os.Exit(1)
	}

	logger.Default.SetLevel(logger.ParseLevel(cfg.LogLevel))
	logger.I("Запуск приложения (%s)", config.LoadedFrom)

	ctx := context.Background()

	c, err := di.New(ctx, cfg)
	if err != nil {
		logger.E("Сборка приложения: %v", err)
		os.Exit(1)
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
		os.Exit(1)
	}

	logger.I("Сервер запущен на %s", addr)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			logger.E("Ошибка работы сервера: %v", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	grpcServer.GracefulStop()
	logger.I("Сервер остановлен")
}
