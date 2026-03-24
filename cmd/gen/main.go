package main

import (
	"context"
	"fmt"
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
	"github.com/magomedcoder/gen/internal/handler"
	"github.com/magomedcoder/gen/internal/repository/postgres"
	"github.com/magomedcoder/gen/internal/runner"
	"github.com/magomedcoder/gen/internal/service"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		logger.Default.SetLevel(logger.LevelInfo)
		logger.E("Ошибка загрузки конфигурации: %v", err)
		os.Exit(1)
	}

	logger.Default.SetLevel(logger.ParseLevel(cfg.Log.Level))
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
	db, err := postgres.NewDB(ctx, dsn)
	if err != nil {
		logger.E("Ошибка подключения к базе данных: %v", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.I("Подключение к базе данных установлено")

	if err := bootstrap.RunMigrations(ctx, db, gen.Postgres); err != nil {
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
	fileRepo := postgres.NewFileRepository(db)

	jwtService := service.NewJWTService(cfg)

	if err := bootstrap.CreateFirstUser(ctx, userRepo, jwtService); err != nil {
		logger.E("Ошибка создания первого пользователя: %v", err)
		os.Exit(1)
	}
	logger.D("Первый пользователь проверен/создан")

	authUseCase := usecase.NewAuthUseCase(userRepo, tokenRepo, jwtService)

	var initialRunners []runner.RunnerState
	for _, e := range cfg.Runners.Entries {
		if a := strings.TrimSpace(e.Address); a != "" {
			initialRunners = append(initialRunners, runner.RunnerState{
				Address: a,
				Name:    strings.TrimSpace(e.Name),
				Enabled: true,
			})
		}
	}
	if len(initialRunners) == 0 {
		logger.I("Раннеры только по саморегистрации (токены из runners)")
	}

	runnerReg := runner.NewRegistry(initialRunners)
	runnerPool := runner.NewPool(runnerReg)
	defer runnerPool.Close()
	llmRepo := runnerPool

	chatUseCase := usecase.NewChatUseCase(sessionRepo, chatPreferenceRepo, chatSessionSettingsRepo, messageRepo, fileRepo, llmRepo, cfg.Attachments.SaveDir)
	editorUseCase := usecase.NewEditorUseCase(llmRepo, chatPreferenceRepo, editorHistoryRepo)
	userUseCase := usecase.NewUserUseCase(userRepo, tokenRepo, jwtService)

	authHandler := handler.NewAuthHandler(cfg, authUseCase)
	chatHandler := handler.NewChatHandler(chatUseCase, authUseCase)
	editorHandler := handler.NewEditorHandler(editorUseCase, authUseCase)
	userHandler := handler.NewUserHandler(userUseCase, authUseCase)

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
