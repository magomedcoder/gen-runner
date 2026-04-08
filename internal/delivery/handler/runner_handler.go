package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/magomedcoder/gen/api/pb/commonpb"
	"github.com/magomedcoder/gen/api/pb/llmrunnerpb"
	"github.com/magomedcoder/gen/api/pb/runnerpb"
	"github.com/magomedcoder/gen/config"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/service"
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
	registry            *service.Registry
	pool                *service.Pool
	authUseCase         *usecase.AuthUseCase
	cfg                 *config.Config
	runnerRepo          domain.RunnerRepository
	webSearchSettingsUC *usecase.WebSearchSettingsUseCase
}

func NewRunnerHandler(
	registry *service.Registry,
	pool *service.Pool,
	authUseCase *usecase.AuthUseCase,
	cfg *config.Config,
	runnerRepo domain.RunnerRepository,
	webSearchSettingsUC *usecase.WebSearchSettingsUseCase,
) *RunnerHandler {
	return &RunnerHandler{
		registry:            registry,
		pool:                pool,
		authUseCase:         authUseCase,
		cfg:                 cfg,
		runnerRepo:          runnerRepo,
		webSearchSettingsUC: webSearchSettingsUC,
	}
}

func (h *RunnerHandler) syncRegistry(ctx context.Context) error {
	list, err := h.runnerRepo.List(ctx)
	if err != nil {
		return err
	}
	h.registry.ReplaceAll(service.RunnerStatesFromDomain(list))
	return nil
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
			Address:       strings.TrimSpace(r.Address),
			Name:          strings.TrimSpace(r.Name),
			SelectedModel: strings.TrimSpace(r.SelectedModel),
		})
	}

	return &runnerpb.GetUserRunnersResponse{Runners: out}, nil
}

func (h *RunnerHandler) CreateRunner(ctx context.Context, req *runnerpb.CreateRunnerRequest) (*commonpb.Empty, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}
	host, port, err := domain.ParseRunnerHostOrHostPort(req.GetHost(), req.GetPort())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if _, err := h.runnerRepo.Create(ctx, req.GetName(), host, port, req.GetEnabled(), req.GetSelectedModel()); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	if err := h.syncRegistry(ctx); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	logger.I("CreateRunner: %s:%d", host, port)
	return &commonpb.Empty{}, nil
}

func (h *RunnerHandler) UpdateRunner(ctx context.Context, req *runnerpb.UpdateRunnerRequest) (*commonpb.Empty, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if req == nil || req.GetId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "нужен положительный id")
	}
	host, port, err := domain.ParseRunnerHostOrHostPort(req.GetHost(), req.GetPort())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	prev, err := h.runnerRepo.GetByID(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "раннер не найден")
		}
		return nil, ToStatusError(codes.Internal, err)
	}
	oldAddr := domain.RunnerListenAddress(prev.Host, prev.Port)
	_, err = h.runnerRepo.Update(ctx, req.GetId(), req.GetName(), host, port, req.GetEnabled(), req.GetSelectedModel())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "раннер не найден")
		}
		return nil, ToStatusError(codes.Internal, err)
	}
	newAddr := domain.RunnerListenAddress(host, port)
	if oldAddr != "" && newAddr != "" && oldAddr != newAddr {
		h.pool.CloseAddrForget(oldAddr)
	}
	if prev.Enabled && !req.GetEnabled() && oldAddr != "" {
		h.pool.CloseAddr(oldAddr)
	}
	if err := h.syncRegistry(ctx); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	logger.I("UpdateRunner: id=%d", req.GetId())
	return &commonpb.Empty{}, nil
}

func (h *RunnerHandler) DeleteRunner(ctx context.Context, req *runnerpb.DeleteRunnerRequest) (*commonpb.Empty, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if req == nil || req.GetId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "нужен положительный id")
	}
	prev, err := h.runnerRepo.GetByID(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "раннер не найден")
		}
		return nil, ToStatusError(codes.Internal, err)
	}
	addr := domain.RunnerListenAddress(prev.Host, prev.Port)
	h.pool.CloseAddrForget(addr)
	if err := h.runnerRepo.Delete(ctx, req.GetId()); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	if err := h.syncRegistry(ctx); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	logger.I("DeleteRunner: id=%d", req.GetId())
	return &commonpb.Empty{}, nil
}

func (h *RunnerHandler) SetRunnerEnabled(ctx context.Context, req *runnerpb.SetRunnerEnabledRequest) (*commonpb.Empty, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if req == nil || req.GetId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "нужен положительный id")
	}
	before, err := h.runnerRepo.GetByID(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "раннер не найден")
		}
		return nil, ToStatusError(codes.Internal, err)
	}
	if err := h.runnerRepo.SetEnabled(ctx, req.GetId(), req.GetEnabled()); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "раннер не найден")
		}
		return nil, ToStatusError(codes.Internal, err)
	}
	addr := domain.RunnerListenAddress(before.Host, before.Port)
	if !req.GetEnabled() && addr != "" {
		h.pool.CloseAddr(addr)
	}
	if err := h.syncRegistry(ctx); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	logger.I("SetRunnerEnabled: id=%d enabled=%v", req.GetId(), req.GetEnabled())
	return &commonpb.Empty{}, nil
}

func (h *RunnerHandler) GetRunnersStatus(ctx context.Context, _ *commonpb.Empty) (*runnerpb.GetRunnersStatusResponse, error) {
	return &runnerpb.GetRunnersStatusResponse{
		HasActiveRunners: h.registry.HasActiveRunners(),
	}, nil
}

func (h *RunnerHandler) GetRunnerModels(ctx context.Context, req *runnerpb.GetRunnerModelsRequest) (*runnerpb.GetRunnerModelsResponse, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if req == nil || req.GetRunnerId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "нужен положительный id")
	}
	st, ok := h.registry.GetByID(req.GetRunnerId())
	if !ok || strings.TrimSpace(st.Address) == "" {
		return nil, status.Error(codes.NotFound, "раннер не найден")
	}
	models, err := h.pool.GetModelsOnRunner(ctx, st.Address)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	return &runnerpb.GetRunnerModelsResponse{Models: models}, nil
}

func (h *RunnerHandler) RunnerLoadModel(ctx context.Context, req *runnerpb.RunnerLoadModelRequest) (*commonpb.Empty, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if req == nil || req.GetRunnerId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "нужен положительный id")
	}
	model := strings.TrimSpace(req.GetModel())
	if model == "" {
		return nil, status.Error(codes.InvalidArgument, "укажите модель")
	}
	st, ok := h.registry.GetByID(req.GetRunnerId())
	if !ok || strings.TrimSpace(st.Address) == "" {
		return nil, status.Error(codes.NotFound, "раннер не найден")
	}
	if !st.Enabled {
		return nil, status.Error(codes.FailedPrecondition, "включите раннер")
	}
	if err := h.pool.WarmModelOnRunner(ctx, st.Address, model); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	logger.I("RunnerLoadModel: id=%d model=%s", req.GetRunnerId(), model)
	return &commonpb.Empty{}, nil
}

func (h *RunnerHandler) RunnerUnloadModel(ctx context.Context, req *runnerpb.RunnerUnloadModelRequest) (*commonpb.Empty, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if req == nil || req.GetRunnerId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "нужен положительный id")
	}
	st, ok := h.registry.GetByID(req.GetRunnerId())
	if !ok || strings.TrimSpace(st.Address) == "" {
		return nil, status.Error(codes.NotFound, "раннер не найден")
	}
	if err := h.pool.UnloadModelOnRunner(ctx, st.Address); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	logger.I("RunnerUnloadModel: id=%d", req.GetRunnerId())
	return &commonpb.Empty{}, nil
}

func (h *RunnerHandler) RunnerResetMemory(ctx context.Context, req *runnerpb.RunnerResetMemoryRequest) (*commonpb.Empty, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if req == nil || req.GetRunnerId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "нужен положительный id")
	}
	st, ok := h.registry.GetByID(req.GetRunnerId())
	if !ok || strings.TrimSpace(st.Address) == "" {
		return nil, status.Error(codes.NotFound, "раннер не найден")
	}
	addr := strings.TrimSpace(st.Address)
	if err := h.pool.ResetMemoryOnRunner(ctx, addr); err != nil {
		logger.W("RunnerResetMemory: llm-runner: %v", err)
	}
	h.pool.CloseAddr(addr)
	logger.I("RunnerResetMemory: id=%d addr=%s", req.GetRunnerId(), addr)
	return &commonpb.Empty{}, nil
}

func (h *RunnerHandler) GetWebSearchSettings(ctx context.Context, _ *commonpb.Empty) (*runnerpb.WebSearchSettingsResponse, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if h.webSearchSettingsUC == nil {
		return nil, status.Error(codes.Internal, "web search settings unavailable")
	}
	s, err := h.webSearchSettingsUC.Get(ctx)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	return &runnerpb.WebSearchSettingsResponse{
		Settings: &runnerpb.WebSearchSettings{
			Enabled:              s.Enabled,
			MaxResults:           int32(s.MaxResults),
			BraveApiKey:          s.BraveAPIKey,
			GoogleApiKey:         s.GoogleAPIKey,
			GoogleSearchEngineId: s.GoogleSearchEngineID,
			YandexUser:           s.YandexUser,
			YandexKey:            s.YandexKey,
		},
	}, nil
}

func (h *RunnerHandler) UpdateWebSearchSettings(ctx context.Context, req *runnerpb.UpdateWebSearchSettingsRequest) (*runnerpb.WebSearchSettingsResponse, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if h.webSearchSettingsUC == nil {
		return nil, status.Error(codes.Internal, "web search settings unavailable")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}
	s := &domain.WebSearchSettings{
		Enabled:              req.GetEnabled(),
		MaxResults:           int(req.GetMaxResults()),
		BraveAPIKey:          req.GetBraveApiKey(),
		GoogleAPIKey:         req.GetGoogleApiKey(),
		GoogleSearchEngineID: req.GetGoogleSearchEngineId(),
		YandexUser:           req.GetYandexUser(),
		YandexKey:            req.GetYandexKey(),
	}
	if err := h.webSearchSettingsUC.Update(ctx, s); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	return h.GetWebSearchSettings(ctx, &commonpb.Empty{})
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

func (h *RunnerHandler) RegisterRunnerWithToken(ctx context.Context, _ *llmrunnerpb.RunnerRegisterRequest) (*llmrunnerpb.Empty, error) {
	return nil, status.Error(codes.FailedPrecondition, "саморегистрация отключена: добавьте раннер в админке (имя, IP, порт)")
}

func (h *RunnerHandler) UnregisterRunnerWithToken(ctx context.Context, _ *llmrunnerpb.RunnerUnregisterRequest) (*llmrunnerpb.Empty, error) {
	return nil, status.Error(codes.FailedPrecondition, "саморегистрация отключена: удалите раннер в админке")
}
