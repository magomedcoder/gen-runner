package di

import (
	"context"
	"fmt"
	"github.com/magomedcoder/gen"
	"github.com/magomedcoder/gen/internal/config"
	"github.com/magomedcoder/gen/internal/provider"
	"path/filepath"

	"github.com/magomedcoder/gen/api/pb/authpb"
	"github.com/magomedcoder/gen/api/pb/chatpb"
	"github.com/magomedcoder/gen/api/pb/editorpb"
	"github.com/magomedcoder/gen/api/pb/llmrunnerpb"
	"github.com/magomedcoder/gen/api/pb/runnerpb"
	"github.com/magomedcoder/gen/api/pb/userpb"
	"github.com/magomedcoder/gen/internal/bootstrap"
	"github.com/magomedcoder/gen/internal/delivery/handler"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mcpclient"
	"github.com/magomedcoder/gen/internal/repository/postgres"
	"github.com/magomedcoder/gen/internal/service"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/logger"
	"github.com/magomedcoder/gen/pkg/rag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/gorm"
)

type Container struct {
	db             *gorm.DB
	pool           *service.Pool
	fileRepo       domain.FileRepository
	DocumentIngest *usecase.DocumentIngestUseCase
	authHandler    *handler.AuthHandler
	chatHandler    *handler.ChatHandler
	editorHandler  *handler.EditorHandler
	userHandler    *handler.UserHandler
	runnerHandler  *handler.RunnerHandler
}

func New(ctx context.Context, cfg *config.Config) (*Container, error) {
	mcpclient.SetHTTPHostPolicy(func(host string) bool {
		return cfg.MCPHTTPHostAllowed(host)
	})
	if cfg.MCP.HTTPAllowAny {
		logger.W("MCP: mcp.http_allow_any включён - разрешены любые исходящие HTTP(S) хосты (не для продакшена)")
	}

	if roots, err := mcpclient.RootsFromConfigStrings(cfg.MCP.Roots); err != nil {
		logger.W("MCP: некорректный mcp.roots в конфиге: %v", err)
	} else {
		mcpclient.SetSessionRoots(roots)
		if len(roots) > 0 {
			logger.I("MCP: для clients/roots задано корней: %d", len(roots))
		}
	}

	mcpclient.SetSamplingEnabled(cfg.MCP.SamplingEnabled)

	if cfg.MCP.SamplingEnabled {
		logger.W("MCP: sampling_enabled=true - серверы могут запрашивать доп. вызовы LLM во время tools/call (расход токенов)")
	}

	mcpclient.SetLogServerMessages(cfg.MCP.LogServerMessages)

	if cfg.MCP.LogServerMessages {
		logger.I("MCP: log_server_messages=true - уведомления notifications/message от серверов пишутся в журнал")
	}

	mcpclient.SetHTTPReuseSessions(cfg.MCP.HTTPReuseSessions)
	mcpclient.SetHTTPSessionMaxIdleSec(cfg.MCP.HTTPSessionMaxIdleSeconds)

	if cfg.MCP.HTTPReuseSessions {
		logger.I("MCP: http_reuse_sessions=true - для sse/streamable одна сессия на сервер (сброс при ошибке, idle и CRUD); tools/call с sampling открывает отдельную сессию")
	}

	db, err := provider.NewDB(ctx, cfg, cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("подключение к базе данных: %w", err)
	}

	if err := bootstrap.RunMigrations(ctx, db, gen.Postgres); err != nil {
		return nil, fmt.Errorf("миграции: %w", err)
	}

	userRepo := postgres.NewUserRepository(db)
	tokenRepo := postgres.NewUserSessionRepository(db)
	runnerRepo := postgres.NewRunnerRepository(db)
	sessionRepo := postgres.NewChatSessionRepository(db)
	chatPreferenceRepo := postgres.NewChatPreferenceRepository(db, runnerRepo)
	chatSessionSettingsRepo := postgres.NewChatSessionSettingsRepository(db)
	webSearchSettingsRepo := postgres.NewWebSearchSettingsRepository(db)
	mcpServerRepo := postgres.NewMCPServerRepository(db)
	editorHistoryRepo := postgres.NewEditorHistoryRepository(db)
	messageRepo := postgres.NewMessageRepository(db)
	messageEditRepo := postgres.NewMessageEditRepository(db)
	assistantRegenRepo := postgres.NewAssistantMessageRegenerationRepository(db)
	fileRepo := postgres.NewFileRepository(db)
	documentRAGRepo := postgres.NewDocumentRAGRepository(db)

	jwtService := service.NewJWTService(cfg)

	if err := bootstrap.CreateFirstUser(ctx, userRepo, jwtService); err != nil {
		closeGormDB(db)
		return nil, fmt.Errorf("первый пользователь: %w", err)
	}
	logger.D("Первый пользователь проверен/создан")

	authTxRunner := postgres.NewAuthTransactionRunner(db)
	chatTxRunner := postgres.NewChatTransactionRunner(db, runnerRepo)

	authUseCase := usecase.NewAuthUseCase(authTxRunner, userRepo, tokenRepo, jwtService)

	runnerRows, err := runnerRepo.List(ctx)
	if err != nil {
		closeGormDB(db)
		return nil, fmt.Errorf("загрузка раннеров: %w", err)
	}
	runnerReg := service.NewRegistry(service.RunnerStatesFromDomain(runnerRows))
	runnerPool := service.NewPool(runnerReg)
	llmRepo := runnerPool

	webSearchSettingsUC := usecase.NewWebSearchSettingsUseCase(webSearchSettingsRepo)
	mcpServersUC := usecase.NewMCPServersUseCase(mcpServerRepo)
	mcpToolsListCache := mcpclient.NewToolsListCache()
	mcpclient.SetToolsListCacheForNotifications(mcpToolsListCache)

	documentIngestUC := usecase.NewDocumentIngestUseCase(
		sessionRepo, fileRepo, documentRAGRepo, runnerRepo, llmRepo, filepath.Join(cfg.DataDir, "uploads"),
		rag.SplitOptions{
			ChunkSizeRunes:    cfg.RAG.EffectiveChunkSizeRunes(),
			ChunkOverlapRunes: cfg.RAG.EffectiveChunkOverlapRunes(),
		},
		cfg.RAG.EffectiveEmbedBatchSize(),
		cfg.RAG.EffectiveMaxChunkEmbedRunes(),
	)

	chatUseCase := usecase.NewChatUseCase(chatTxRunner, sessionRepo, chatPreferenceRepo, chatSessionSettingsRepo, messageRepo, messageEditRepo, assistantRegenRepo, fileRepo, runnerRepo, llmRepo, runnerPool, runnerReg, filepath.Join(cfg.DataDir, "uploads"), cfg.AttachmentHydrateParallelism, webSearchSettingsRepo, mcpServerRepo, mcpToolsListCache, documentIngestUC, cfg.RAG.BackgroundIndexTimeout(), cfg.RAG.EffectiveLLMContextFallbackTokens(), cfg.RAG.EffectiveMaxExtractedRunesOnUpload())
	editorUseCase := usecase.NewEditorUseCase(llmRepo, editorHistoryRepo, runnerRepo)
	userUseCase := usecase.NewUserUseCase(userRepo, tokenRepo, jwtService)

	return &Container{
		db:             db,
		pool:           runnerPool,
		fileRepo:       fileRepo,
		DocumentIngest: documentIngestUC,
		authHandler:    handler.NewAuthHandler(cfg, authUseCase),
		chatHandler:    handler.NewChatHandler(cfg, chatUseCase, authUseCase, documentIngestUC),
		editorHandler:  handler.NewEditorHandler(editorUseCase, authUseCase),
		userHandler:    handler.NewUserHandler(userUseCase, authUseCase),
		runnerHandler:  handler.NewRunnerHandler(runnerReg, runnerPool, authUseCase, cfg, runnerRepo, webSearchSettingsUC, mcpServersUC, mcpToolsListCache),
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
	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func closeGormDB(g *gorm.DB) {
	if g == nil {
		return
	}
	sqlDB, err := g.DB()
	if err != nil {
		return
	}
	_ = sqlDB.Close()
}
