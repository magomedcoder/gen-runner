package main

import (
	"context"
	"github.com/magomedcoder/gen/api/pb/llm-runner/llmrunnerpb"
	runner "github.com/magomedcoder/gen/llm-runner"
	"github.com/magomedcoder/gen/llm-runner/config"
	"github.com/magomedcoder/gen/llm-runner/gpu"
	"github.com/magomedcoder/gen/llm-runner/provider"
	"github.com/magomedcoder/gen/pkg/logger"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"net"
	"os/signal"
	"syscall"
	"time"
)

func cmdServe() *cli.Command {
	return &cli.Command{
		Name:    "serve",
		Aliases: []string{"s"},
		Usage:   "Запустить сервер раннера",
		Action:  runServe,
	}
}

func runServe(ctx context.Context, _ *cli.Command) error {
	ctx, stopSignals := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	cfg, err := config.Load("./configs/config-llm-runner.yaml")
	if err != nil {
		logger.Default.SetLevel(logger.LevelInfo)
		logger.E("Ошибка загрузки конфигурации: %v", err)
		return err
	}

	logger.Default.SetLevel(logger.ParseLevel(cfg.LogLevel))
	logger.I("Запуск раннера")
	logger.I("Конфиг: gpu_layers=%d (0 - только CPU; -1 - все слои на GPU; нужны make build-gpu и CUDA-сборка libllama)", cfg.GpuLayers)

	textProvider, err := provider.NewTextProvider(cfg)
	if err != nil {
		logger.E("Движок текста: %v", err)
		return err
	}

	gpuCollector := gpu.NewCollector()

	runnerServer := runner.NewServer(textProvider, gpuCollector, cfg.MaxConcurrentGenerations, cfg.UnloadAfterRPC(), cfg.ModelsDir())

	listenAddr := cfg.ListenAddr()
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		logger.E("Ошибка слушателя: %v", err)
		return err
	}
	defer lis.Close()

	grpcServer := grpc.NewServer(
		grpc.MaxConcurrentStreams(1024),
	)
	llmrunnerpb.RegisterLLMRunnerServiceServer(grpcServer, runnerServer)

	errCh := make(chan error, 1)
	go func() {
		logger.I("Раннер слушает на %s", listenAddr)
		errCh <- grpcServer.Serve(lis)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err == nil || err == grpc.ErrServerStopped {
			return nil
		}
		logger.E("Ошибка gRPC: %v", err)
		return err
	}

	logger.I("Остановка раннера...")

	const shutdownTimeout = 15 * time.Second
	done := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		logger.W("GracefulStop не завершился за %s, принудительная остановка", shutdownTimeout)
		grpcServer.Stop()
	}

	unloadCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := textProvider.UnloadModel(unloadCtx); err != nil {
		logger.W("Выгрузка модели при остановке: %v", err)
	}

	logger.I("Раннер остановлен")
	return nil
}
