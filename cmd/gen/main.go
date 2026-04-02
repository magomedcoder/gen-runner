package main

import (
	"context"
	"fmt"
	"github.com/magomedcoder/gen/internal/delivery/handler"
	"github.com/magomedcoder/gen/internal/provider"
	"strings"

	"github.com/magomedcoder/gen"
	"github.com/magomedcoder/gen/api/pb/authpb"
	"github.com/magomedcoder/gen/api/pb/chatpb"
	"github.com/magomedcoder/gen/api/pb/editorpb"
	"github.com/magomedcoder/gen/api/pb/llmrunnerpb"
	"github.com/magomedcoder/gen/api/pb/runnerpb"
	"github.com/magomedcoder/gen/api/pb/userpb"
	"github.com/magomedcoder/gen/config"
	"github.com/magomedcoder/gen/internal/bootstrap"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/repository/postgres"
	"github.com/magomedcoder/gen/internal/service"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		logger.Default.SetLevel(logger.LevelInfo)
		logger.E("Ошибка загрузки конфигурации: %v", err)
		os.Exit(1)
	}

	logger.Default.SetLevel(logger.ParseLevel(cfg.LogLevel))
	logger.I("Запуск приложения (%s)", config.LoadedFrom)

	ctx := context.Background()

	if err := bootstrap.CheckDatabase(ctx, cfg.Database); err != nil {
		logger.E("Ошибка инициализации базы данных: %v", err)
		os.Exit(1)
	}
	logger.D("База данных доступна")

	dsn, err := cfg.Database.PostgresDSN()
	if err != nil {
		logger.E("Ошибка конфигурации базы данных: %v", err)
		os.Exit(1)
	}
	db, err := provider.NewDB(ctx, dsn)
	if err != nil {
		logger.E("Ошибка подключения к базе данных: %v", err)
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err != nil {
		logger.E("Ошибка получения sql.DB: %v", err)
		os.Exit(1)
	}
	defer sqlDB.Close()
	logger.I("Подключение к базе данных установлено")

	if err := bootstrap.RunMigrations(ctx, sqlDB, gen.Postgres); err != nil {
		logger.E("Ошибка применения миграций: %v", err)
		os.Exit(1)
	}
	logger.D("Миграции применены")

	userRepo := postgres.NewUserRepository(db)
	tokenRepo := postgres.NewUserSessionRepository(db)
	sessionRepo := postgres.NewChatSessionRepository(db)
	chatPreferenceRepo := postgres.NewChatPreferenceRepository(db)
	chatSessionSettingsRepo := postgres.NewChatSessionSettingsRepository(db)
	editorHistoryRepo := postgres.NewEditorHistoryRepository(db)
	messageRepo := postgres.NewMessageRepository(db)
	messageEditRepo := postgres.NewMessageEditRepository(db)
	assistantRegenRepo := postgres.NewAssistantMessageRegenerationRepository(db)
	fileRepo := postgres.NewFileRepository(db)

	jwtService := service.NewJWTService(cfg)

	if err := bootstrap.CreateFirstUser(ctx, userRepo, jwtService); err != nil {
		logger.E("Ошибка создания первого пользователя: %v", err)
		os.Exit(1)
	}
	logger.D("Первый пользователь проверен/создан")

	authTxRunner := postgres.NewAuthTransactionRunner(db)
	chatTxRunner := postgres.NewChatTransactionRunner(db)

	authUseCase := usecase.NewAuthUseCase(authTxRunner, userRepo, tokenRepo, jwtService)

	var initialRunners []service.RunnerState
	for _, e := range cfg.Runners.Entries {
		if a := strings.TrimSpace(e.Address); a != "" {
			initialRunners = append(initialRunners, service.RunnerState{
				Address: a,
				Name:    strings.TrimSpace(e.Name),
				Enabled: true,
			})
		}
	}
	if len(initialRunners) == 0 {
		logger.I("Раннеры только по саморегистрации (токены из runners)")
	}

	runnerReg := service.NewRegistry(initialRunners)
	runnerPool := service.NewPool(runnerReg)
	defer runnerPool.Close()
	llmRepo := runnerPool

	chatUseCase := usecase.NewChatUseCase(chatTxRunner, sessionRepo, chatPreferenceRepo, chatSessionSettingsRepo, messageRepo, messageEditRepo, assistantRegenRepo, fileRepo, llmRepo, runnerPool, cfg.UploadDir, cfg.DefaultRunnerAddress())
	editorUseCase := usecase.NewEditorUseCase(llmRepo, chatPreferenceRepo, editorHistoryRepo, cfg.DefaultRunnerAddress())
	userUseCase := usecase.NewUserUseCase(userRepo, tokenRepo, jwtService)

	authHandler := handler.NewAuthHandler(cfg, authUseCase)
	chatHandler := handler.NewChatHandler(chatUseCase, authUseCase)
	editorHandler := handler.NewEditorHandler(editorUseCase, authUseCase)
	userHandler := handler.NewUserHandler(userUseCase, authUseCase)

	go runSessionFileTTLCleanup(fileRepo)

	grpcServer := grpc.NewServer()

	runnerHandler := handler.NewRunnerHandler(runnerReg, runnerPool, authUseCase, cfg)
	runnerpb.RegisterRunnerServiceServer(grpcServer, runnerHandler)
	llmrunnerpb.RegisterLLMRunnerServiceServer(grpcServer, runnerHandler)

	authpb.RegisterAuthServiceServer(grpcServer, authHandler)
	chatpb.RegisterChatServiceServer(grpcServer, chatHandler)
	editorpb.RegisterEditorServiceServer(grpcServer, editorHandler)
	userpb.RegisterUserServiceServer(grpcServer, userHandler)

	reflection.Register(grpcServer)

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

func runSessionFileTTLCleanup(fileRepo domain.FileRepository) {
	tick := time.NewTicker(10 * time.Minute)
	defer tick.Stop()
	for range tick.C {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		n, err := fileRepo.DeleteExpired(ctx)
		cancel()
		if err != nil {
			logger.W("очистка файлов по TTL: %v", err)
			continue
		}

		if n > 0 {
			logger.I("удалено просроченных файлов: %d", n)
		}
	}
}
