package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/magomedcoder/llm-runner"
	"github.com/magomedcoder/llm-runner/config"
	"github.com/magomedcoder/llm-runner/gpu"
	"github.com/magomedcoder/llm-runner/logger"
	"github.com/magomedcoder/llm-runner/pb"
	"github.com/magomedcoder/llm-runner/provider"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		logger.Default.SetLevel(logger.LevelInfo)
		logger.E("config: %v", err)
		os.Exit(1)
	}
	logger.Default.SetLevel(logger.ParseLevel(cfg.Log.Level))

	textProvider, err := provider.NewTextProvider(cfg)
	if err != nil {
		logger.E("text provider: %v", err)
		os.Exit(1)
	}

	gpuCollector := gpu.NewCollector()
	runnerServer := runner.NewServer(textProvider, gpuCollector, cfg.MaxConcurrentGenerations)
	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		logger.E("listen: %v", err)
		os.Exit(1)
	}

	defer lis.Close()

	grpcServer := grpc.NewServer()

	pb.RegisterLLMRunnerServiceServer(grpcServer, runnerServer)
	go func() {
		logger.I("listening on %s", cfg.ListenAddr)
		if err := grpcServer.Serve(lis); err != nil {
			logger.E("grpc: %v", err)
			os.Exit(1)
		}
	}()

	if cfg.CoreAddr != "" && cfg.ListenAddr != "" {
		if err := registerWithCore(cfg.CoreAddr, cfg.ListenAddr, cfg.RegistrationToken); err != nil {
			logger.W("Не удалось зарегистрировать ядро: %v", err)
		} else {
			logger.I("зарегистрировано в ядре %s как %s", cfg.CoreAddr, cfg.ListenAddr)
			defer unregisterFromCore(cfg.CoreAddr, cfg.ListenAddr, cfg.RegistrationToken)
		}
	}
	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	grpcServer.GracefulStop()
}

func registerWithCore(coreAddr, registerAddress, registrationToken string) error {
	conn, err := grpc.NewClient(coreAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("подключиться к ядру: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctx = outgoingContextWithRunnerToken(ctx, registrationToken)
	client := pb.NewLLMRunnerServiceClient(conn)
	_, err = client.Register(ctx, &pb.RegisterRunnerRequest{
		Address: registerAddress,
	})

	return err
}

func unregisterFromCore(coreAddr, registerAddress, registrationToken string) {
	conn, err := grpc.NewClient(coreAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.W("отменить регистрацию подключения: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctx = outgoingContextWithRunnerToken(ctx, registrationToken)
	client := pb.NewLLMRunnerServiceClient(conn)
	_, _ = client.Unregister(ctx, &pb.UnregisterRunnerRequest{
		Address: registerAddress,
	})
}

func outgoingContextWithRunnerToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}

	return metadata.AppendToOutgoingContext(ctx, runner.MetadataRunnerToken, token)
}
