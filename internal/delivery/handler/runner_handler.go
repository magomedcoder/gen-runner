package handler

import (
	"context"
	"github.com/magomedcoder/gen/internal/service"
	"strings"

	"github.com/magomedcoder/gen/api/pb/commonpb"
	"github.com/magomedcoder/gen/api/pb/llmrunnerpb"
	"github.com/magomedcoder/gen/api/pb/runnerpb"
	"github.com/magomedcoder/gen/config"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const grpcMetadataRunnerAddress = "runner-address"

type RunnerHandler struct {
	runnerpb.UnimplementedRunnerServiceServer
	llmrunnerpb.UnimplementedLLMRunnerServiceServer
	registry    *service.Registry
	pool        *service.Pool
	authUseCase *usecase.AuthUseCase
	cfg         *config.Config
}

func NewRunnerHandler(registry *service.Registry, pool *service.Pool, authUseCase *usecase.AuthUseCase, cfg *config.Config) *RunnerHandler {
	return &RunnerHandler{
		registry:    registry,
		pool:        pool,
		authUseCase: authUseCase,
		cfg:         cfg,
	}
}

func (h *RunnerHandler) GetRunners(ctx context.Context, _ *commonpb.Empty) (*runnerpb.GetRunnersResponse, error) {
	logger.D("GetRunners: запрос списка раннеров")
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}

	runners := h.registry.GetRunners()
	for _, r := range runners {
		if !r.Enabled {
			r.Connected = false
			continue
		}

		conn, gpuList, si, loaded := h.pool.ProbeLLMRunner(ctx, r.Address)
		r.Connected = conn
		if len(gpuList) > 0 {
			r.Gpus = gpuList
		}

		if si != nil {
			r.ServerInfo = si
		}
		if loaded != nil {
			r.LoadedModel = loaded
		}
	}
	logger.V("GetRunners: возвращено раннеров: %d", len(runners))
	return &runnerpb.GetRunnersResponse{
		Runners: runners,
	}, nil
}

func (h *RunnerHandler) GetUserRunners(ctx context.Context, _ *commonpb.Empty) (*runnerpb.GetUserRunnersResponse, error) {
	if _, err := GetUserFromContext(ctx, h.authUseCase); err != nil {
		return nil, err
	}

	all := h.registry.GetRunners()
	out := make([]*runnerpb.UserRunnerInfo, 0, len(all))
	for _, r := range all {
		if r == nil || !r.Enabled || strings.TrimSpace(r.Address) == "" {
			continue
		}

		out = append(out, &runnerpb.UserRunnerInfo{
			Address: strings.TrimSpace(r.Address),
			Name:    strings.TrimSpace(r.Name),
		})
	}

	return &runnerpb.GetUserRunnersResponse{Runners: out}, nil
}

func (h *RunnerHandler) SetRunnerEnabled(ctx context.Context, req *runnerpb.SetRunnerEnabledRequest) (*commonpb.Empty, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if req != nil {
		h.registry.SetEnabled(req.Address, req.Enabled)
		if !req.Enabled {
			h.pool.CloseAddr(req.Address)
		}
		logger.I("SetRunnerEnabled: адрес=%s enabled=%v", req.Address, req.Enabled)
	}

	return &commonpb.Empty{}, nil
}

func (h *RunnerHandler) GetRunnersStatus(ctx context.Context, _ *commonpb.Empty) (*runnerpb.GetRunnersStatusResponse, error) {
	return &runnerpb.GetRunnersStatusResponse{
		HasActiveRunners: h.registry.HasActiveRunners(),
	}, nil
}

func (h *RunnerHandler) RegisterRunner(ctx context.Context, req *runnerpb.RegisterRunnerRequest) (*commonpb.Empty, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if req == nil || strings.TrimSpace(req.GetAddress()) == "" {
		return nil, status.Error(codes.InvalidArgument, "address обязателен")
	}
	addr := strings.TrimSpace(req.GetAddress())
	h.registry.Register(addr)
	logger.I("RegisterRunner: %s", addr)
	return &commonpb.Empty{}, nil
}

func (h *RunnerHandler) UnregisterRunner(ctx context.Context, req *runnerpb.UnregisterRunnerRequest) (*commonpb.Empty, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if req == nil || strings.TrimSpace(req.GetAddress()) == "" {
		return nil, status.Error(codes.InvalidArgument, "address обязателен")
	}
	addr := strings.TrimSpace(req.GetAddress())
	h.pool.CloseAddrForget(addr)
	h.registry.Unregister(addr)
	logger.I("UnregisterRunner: %s", addr)
	return &commonpb.Empty{}, nil
}

func runnerAddressFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.InvalidArgument, "нужны gRPC-метаданные с ключом runner-address")
	}

	vals := md.Get(grpcMetadataRunnerAddress)
	if len(vals) == 0 || strings.TrimSpace(vals[0]) == "" {
		return "", status.Error(codes.InvalidArgument, "метаданные runner-address обязательны (host:port llm-runner)")
	}

	return strings.TrimSpace(vals[0]), nil
}

func (h *RunnerHandler) requireAdminAndRunnerAddr(ctx context.Context) (string, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return "", err
	}

	return runnerAddressFromMetadata(ctx)
}

func (h *RunnerHandler) CheckConnection(ctx context.Context, _ *llmrunnerpb.Empty) (*llmrunnerpb.ConnectionResponse, error) {
	addr, err := h.requireAdminAndRunnerAddr(ctx)
	if err != nil {
		return nil, err
	}

	ok, _, _, _ := h.pool.ProbeLLMRunner(ctx, addr)
	return &llmrunnerpb.ConnectionResponse{
		IsConnected: ok,
	}, nil
}

func (h *RunnerHandler) validateRegistrationToken(provided string) error {
	if h.cfg == nil || len(h.cfg.Runners.Entries) == 0 {
		return status.Error(codes.FailedPrecondition, "саморегистрация отключена: нет записей runners в конфигурации")
	}

	given := strings.TrimSpace(provided)
	if given == "" {
		return status.Error(codes.PermissionDenied, "неверный registration_token")
	}

	if h.cfg.EntryMatchingRegistrationToken(given) == nil {
		return status.Error(codes.PermissionDenied, "неверный registration_token")
	}

	return nil
}

func (h *RunnerHandler) RegisterRunnerWithToken(ctx context.Context, req *llmrunnerpb.RunnerRegisterRequest) (*llmrunnerpb.Empty, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}

	addr := strings.TrimSpace(req.GetListenAddress())
	if addr == "" {
		return nil, status.Error(codes.InvalidArgument, "listen_address обязателен")
	}

	if err := h.validateRegistrationToken(req.GetRegistrationToken()); err != nil {
		return nil, err
	}

	entry := h.cfg.EntryMatchingRegistrationToken(req.GetRegistrationToken())
	if entry == nil {
		return nil, status.Error(codes.PermissionDenied, "неверный registration_token")
	}

	h.registry.RegisterWithName(addr, entry.Name)
	logger.I("RegisterRunnerWithToken: %s", addr)

	return &llmrunnerpb.Empty{}, nil
}

func (h *RunnerHandler) UnregisterRunnerWithToken(ctx context.Context, req *llmrunnerpb.RunnerUnregisterRequest) (*llmrunnerpb.Empty, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}

	addr := strings.TrimSpace(req.GetListenAddress())
	if addr == "" {
		return nil, status.Error(codes.InvalidArgument, "listen_address обязателен")
	}

	if err := h.validateRegistrationToken(req.GetRegistrationToken()); err != nil {
		return nil, err
	}

	h.pool.CloseAddrForget(addr)
	h.registry.Unregister(addr)
	logger.I("UnregisterRunnerWithToken: %s", addr)

	return &llmrunnerpb.Empty{}, nil
}
