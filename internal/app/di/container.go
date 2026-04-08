package di

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/magomedcoder/gen"
	"github.com/magomedcoder/gen/api/pb/authpb"
	"github.com/magomedcoder/gen/api/pb/chatpb"
	"github.com/magomedcoder/gen/api/pb/editorpb"
	"github.com/magomedcoder/gen/api/pb/llmrunnerpb"
	"github.com/magomedcoder/gen/api/pb/runnerpb"
	"github.com/magomedcoder/gen/api/pb/userpb"
	"github.com/magomedcoder/gen/config"
	"github.com/magomedcoder/gen/internal/bootstrap"
	"github.com/magomedcoder/gen/internal/delivery/handler"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/provider"
	"github.com/magomedcoder/gen/internal/repository/postgres"
	"github.com/magomedcoder/gen/internal/service"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type Container struct {
	sqlDB         *sql.DB
	pool          *service.Pool
	fileRepo      domain.FileRepository
	authHandler   *handler.AuthHandler
	chatHandler   *handler.ChatHandler
	editorHandler *handler.EditorHandler
	userHandler   *handler.UserHandler
	runnerHandler *handler.RunnerHandler
}

func New(ctx context.Context, cfg *config.Config) (*Container, error) {
	if err := bootstrap.CheckDatabase(ctx, cfg.Database); err != nil {
		return nil, fmt.Errorf("инициализация базы данных: %w", err)
	}
	logger.D("База данных доступна")

	gormDB, err := provider.NewDB(ctx, &cfg.Database, cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("подключение к базе данных: %w", err)
	}
	logger.I("Подключение к базе данных установлено")

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("получение sql.DB: %w", err)
	}

	if err := bootstrap.RunMigrations(ctx, sqlDB, gen.Postgres); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("миграции: %w", err)
	}
	logger.D("Миграции применены")

	userRepo := postgres.NewUserRepository(gormDB)
	tokenRepo := postgres.NewUserSessionRepository(gormDB)
	runnerRepo := postgres.NewRunnerRepository(gormDB)
	sessionRepo := postgres.NewChatSessionRepository(gormDB)
	chatPreferenceRepo := postgres.NewChatPreferenceRepository(gormDB, runnerRepo)
	chatSessionSettingsRepo := postgres.NewChatSessionSettingsRepository(gormDB)
	webSearchSettingsRepo := postgres.NewWebSearchSettingsRepository(gormDB)
	editorHistoryRepo := postgres.NewEditorHistoryRepository(gormDB)
	messageRepo := postgres.NewMessageRepository(gormDB)
	messageEditRepo := postgres.NewMessageEditRepository(gormDB)
	assistantRegenRepo := postgres.NewAssistantMessageRegenerationRepository(gormDB)
	fileRepo := postgres.NewFileRepository(gormDB)

	jwtService := service.NewJWTService(cfg)

	if err := bootstrap.CreateFirstUser(ctx, userRepo, jwtService); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("первый пользователь: %w", err)
	}
	logger.D("Первый пользователь проверен/создан")

	authTxRunner := postgres.NewAuthTransactionRunner(gormDB)
	chatTxRunner := postgres.NewChatTransactionRunner(gormDB, runnerRepo)

	authUseCase := usecase.NewAuthUseCase(authTxRunner, userRepo, tokenRepo, jwtService)

	runnerRows, err := runnerRepo.List(ctx)
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("загрузка раннеров: %w", err)
	}
	runnerReg := service.NewRegistry(service.RunnerStatesFromDomain(runnerRows))
	runnerPool := service.NewPool(runnerReg)
	llmRepo := runnerPool

	webSearchSettingsUC := usecase.NewWebSearchSettingsUseCase(webSearchSettingsRepo)

	chatUseCase := usecase.NewChatUseCase(chatTxRunner, sessionRepo, chatPreferenceRepo, chatSessionSettingsRepo, messageRepo, messageEditRepo, assistantRegenRepo, fileRepo, runnerRepo, llmRepo, runnerPool, runnerReg, filepath.Join(cfg.DataDir, "uploads"), cfg.DefaultRunnerAddress(), cfg.AttachmentHydrateParallelism, webSearchSettingsRepo)
	editorUseCase := usecase.NewEditorUseCase(llmRepo, editorHistoryRepo, runnerRepo)
	userUseCase := usecase.NewUserUseCase(userRepo, tokenRepo, jwtService)

	return &Container{
		sqlDB:         sqlDB,
		pool:          runnerPool,
		fileRepo:      fileRepo,
		authHandler:   handler.NewAuthHandler(cfg, authUseCase),
		chatHandler:   handler.NewChatHandler(cfg, chatUseCase, authUseCase),
		editorHandler: handler.NewEditorHandler(editorUseCase, authUseCase),
		userHandler:   handler.NewUserHandler(userUseCase, authUseCase),
		runnerHandler: handler.NewRunnerHandler(runnerReg, runnerPool, authUseCase, cfg, runnerRepo, webSearchSettingsUC),
	}, nil
}

func (c *Container) RegisterGRPC(s *grpc.Server) {
	runnerpb.RegisterRunnerServiceServer(s, c.runnerHandler)
	llmrunnerpb.RegisterLLMRunnerServiceServer(s, c.runnerHandler)

	authpb.RegisterAuthServiceServer(s, c.authHandler)
	chatpb.RegisterChatServiceServer(s, c.chatHandler)
	editorpb.RegisterEditorServiceServer(s, c.editorHandler)
	userpb.RegisterUserServiceServer(s, c.userHandler)
	reflection.Register(s)
}

func (c *Container) Close() error {
	c.pool.Close()
	return c.sqlDB.Close()
}
