package internal

import (
	"fmt"
	"log"
	"net"

	"github.com/magomedcoder/gen/internal/service"
	"github.com/magomedcoder/gen/tools/b24-llm-server/api/pb/b24llmpb"

	"google.golang.org/grpc"
)

type App struct {
	llm       *service.LLMRunnerService
	cfg       Config
	respCache *responseCache
}

func New() (*App, error) {
	cfg := ServerConfig()

	llmSvc, err := service.NewLLMRunnerService(cfg.RunnerAddr, cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("инициализация llm runner: %w", err)
	}

	return &App{
		llm:       llmSvc,
		cfg:       cfg,
		respCache: newResponseCache(cfg),
	}, nil
}

func (a *App) Close() {
	if a == nil || a.llm == nil {
		return
	}
	_ = a.llm.Close()
}

func (a *App) Run() error {
	lis, err := net.Listen("tcp", a.cfg.Addr)
	if err != nil {
		return fmt.Errorf("grpc listen %s: %w", a.cfg.Addr, err)
	}

	maxRecv := int(a.cfg.MaxSummarizeBatchBodyBytes)
	if maxRecv <= 0 {
		maxRecv = 20 << 20
	}

	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(maxRecv),
		grpc.MaxSendMsgSize(maxRecv),
	)

	b24llmpb.RegisterB24LLMServer(srv, &b24GRPCServer{app: a})

	if a.respCache.enabled() {
		log.Printf("b24-llm-server gRPC %s, runner=%s, model=%s, analyze_cache=on (ttl=%v max_keys=%d)", a.cfg.Addr, a.cfg.RunnerAddr, a.cfg.Model, a.respCache.ttl, a.respCache.max)
	} else {
		log.Printf("b24-llm-server gRPC %s, runner=%s, model=%s", a.cfg.Addr, a.cfg.RunnerAddr, a.cfg.Model)
	}

	return srv.Serve(lis)
}
