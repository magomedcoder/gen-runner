package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/magomedcoder/gen/internal/config"
	"strings"
	"time"

	"github.com/magomedcoder/gen/api/pb/app/commonpb"
	"github.com/magomedcoder/gen/api/pb/app/runnerpb"
	"github.com/magomedcoder/gen/api/pb/llm-runner/llmrunnerpb"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mcpclient"
	"github.com/magomedcoder/gen/internal/service"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

const grpcMetadataRunnerAddress = "runner-address"

const getRunnersProbeTimeout = 5 * time.Second

func (h *RunnerHandler) validateMCPServerFull(d *domain.MCPServer) error {
	domain.NormalizeMCPServer(d)
	if err := domain.ValidateMCPServerStructure(d); err != nil {
		return err
	}

	return h.cfg.ValidateMCPServer(d)
}

type RunnerHandler struct {
	runnerpb.UnimplementedRunnerServiceServer
	llmrunnerpb.UnimplementedLLMRunnerServiceServer
	registry            *service.Registry
	pool                *service.Pool
	authUseCase         *usecase.AuthUseCase
	cfg                 *config.Config
	runnerRepo          domain.RunnerRepository
	webSearchSettingsUC *usecase.WebSearchSettingsUseCase
	mcpServersUC        *usecase.MCPServersUseCase
	mcpToolsListCache   *mcpclient.ToolsListCache
}

func NewRunnerHandler(
	registry *service.Registry,
	pool *service.Pool,
	authUseCase *usecase.AuthUseCase,
	cfg *config.Config,
	runnerRepo domain.RunnerRepository,
	webSearchSettingsUC *usecase.WebSearchSettingsUseCase,
	mcpServersUC *usecase.MCPServersUseCase,
	mcpToolsListCache *mcpclient.ToolsListCache,
) *RunnerHandler {
	return &RunnerHandler{
		registry:            registry,
		pool:                pool,
		authUseCase:         authUseCase,
		cfg:                 cfg,
		runnerRepo:          runnerRepo,
		webSearchSettingsUC: webSearchSettingsUC,
		mcpServersUC:        mcpServersUC,
		mcpToolsListCache:   mcpToolsListCache,
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

		probeCtx, cancel := context.WithTimeout(ctx, getRunnersProbeTimeout)
		conn, gpuList, si, loaded := h.pool.ProbeLLMRunner(probeCtx, r.Address)
		cancel()
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
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_EMPTY_REQUEST", "пустой запрос")
	}

	host, port, err := domain.ParseRunnerHostOrHostPort(req.GetHost(), req.GetPort())
	if err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_INVALID_HOST_PORT", err.Error())
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
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}
	host, port, err := domain.ParseRunnerHostOrHostPort(req.GetHost(), req.GetPort())
	if err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_INVALID_HOST_PORT", err.Error())
	}
	prev, err := h.runnerRepo.GetByID(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "RUNNER_NOT_FOUND", "раннер не найден")
		}
		return nil, ToStatusError(codes.Internal, err)
	}
	oldAddr := domain.RunnerListenAddress(prev.Host, prev.Port)
	_, err = h.runnerRepo.Update(ctx, req.GetId(), req.GetName(), host, port, req.GetEnabled(), req.GetSelectedModel())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "RUNNER_NOT_FOUND", "раннер не найден")
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
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}
	prev, err := h.runnerRepo.GetByID(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "RUNNER_NOT_FOUND", "раннер не найден")
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
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}
	st, ok := h.registry.GetByID(req.GetRunnerId())
	if !ok || strings.TrimSpace(st.Address) == "" {
		return nil, StatusErrorWithReason(codes.NotFound, "RUNNER_NOT_FOUND", "раннер не найден")
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
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}
	model := strings.TrimSpace(req.GetModel())
	if model == "" {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_MODEL_REQUIRED", "укажите модель")
	}
	st, ok := h.registry.GetByID(req.GetRunnerId())
	if !ok || strings.TrimSpace(st.Address) == "" {
		return nil, StatusErrorWithReason(codes.NotFound, "RUNNER_NOT_FOUND", "раннер не найден")
	}
	if !st.Enabled {
		return nil, StatusErrorWithReason(codes.FailedPrecondition, "RUNNER_MUST_BE_ENABLED", "включите раннер")
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
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}
	st, ok := h.registry.GetByID(req.GetRunnerId())
	if !ok || strings.TrimSpace(st.Address) == "" {
		return nil, StatusErrorWithReason(codes.NotFound, "RUNNER_NOT_FOUND", "раннер не найден")
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
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}
	st, ok := h.registry.GetByID(req.GetRunnerId())
	if !ok || strings.TrimSpace(st.Address) == "" {
		return nil, StatusErrorWithReason(codes.NotFound, "RUNNER_NOT_FOUND", "раннер не найден")
	}
	addr := strings.TrimSpace(st.Address)
	if err := h.pool.ResetMemoryOnRunner(ctx, addr); err != nil {
		logger.W("RunnerResetMemory: gen-runner: %v", err)
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
		return nil, StatusErrorWithReason(codes.Internal, "RUNNER_WEB_SEARCH_UNAVAILABLE", "web search settings unavailable")
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
			YandexEnabled:        s.YandexEnabled,
			GoogleEnabled:        s.GoogleEnabled,
			BraveEnabled:         s.BraveEnabled,
		},
	}, nil
}

func (h *RunnerHandler) UpdateWebSearchSettings(ctx context.Context, req *runnerpb.UpdateWebSearchSettingsRequest) (*runnerpb.WebSearchSettingsResponse, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if h.webSearchSettingsUC == nil {
		return nil, StatusErrorWithReason(codes.Internal, "RUNNER_WEB_SEARCH_UNAVAILABLE", "web search settings unavailable")
	}
	if req == nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_EMPTY_REQUEST", "пустой запрос")
	}
	cur, err := h.webSearchSettingsUC.Get(ctx)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	ye, ge, be := true, true, true
	if cur != nil {
		ye, ge, be = cur.YandexEnabled, cur.GoogleEnabled, cur.BraveEnabled
	}
	if req.YandexEnabled != nil {
		ye = *req.YandexEnabled
	}
	if req.GoogleEnabled != nil {
		ge = *req.GoogleEnabled
	}
	if req.BraveEnabled != nil {
		be = *req.BraveEnabled
	}
	s := &domain.WebSearchSettings{
		Enabled:              req.GetEnabled(),
		MaxResults:           int(req.GetMaxResults()),
		BraveAPIKey:          req.GetBraveApiKey(),
		GoogleAPIKey:         req.GetGoogleApiKey(),
		GoogleSearchEngineID: req.GetGoogleSearchEngineId(),
		YandexUser:           req.GetYandexUser(),
		YandexKey:            req.GetYandexKey(),
		YandexEnabled:        ye,
		GoogleEnabled:        ge,
		BraveEnabled:         be,
	}
	if err := h.webSearchSettingsUC.Update(ctx, s); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	return h.GetWebSearchSettings(ctx, &commonpb.Empty{})
}

func (h *RunnerHandler) GetWebSearchAvailability(ctx context.Context, _ *commonpb.Empty) (*runnerpb.WebSearchAvailabilityResponse, error) {
	if _, err := GetUserFromContext(ctx, h.authUseCase); err != nil {
		return nil, err
	}
	if h.webSearchSettingsUC == nil {
		return &runnerpb.WebSearchAvailabilityResponse{GloballyEnabled: false}, nil
	}
	s, err := h.webSearchSettingsUC.Get(ctx)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}
	return &runnerpb.WebSearchAvailabilityResponse{GloballyEnabled: s.Enabled}, nil
}

func runnerAddressFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", StatusErrorWithReason(codes.InvalidArgument, "RUNNER_METADATA_MISSING", "нужны gRPC-метаданные с ключом runner-address")
	}

	vals := md.Get(grpcMetadataRunnerAddress)
	if len(vals) == 0 || strings.TrimSpace(vals[0]) == "" {
		return "", StatusErrorWithReason(codes.InvalidArgument, "RUNNER_METADATA_ADDRESS_EMPTY", "метаданные runner-address обязательны (host:port gen-runner)")
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

func mapDomainMCPServerToProto(s *domain.MCPServer) *runnerpb.MCPServer {
	if s == nil {
		return nil
	}

	var args []string
	_ = json.Unmarshal([]byte(s.ArgsJSON), &args)

	var env map[string]string
	_ = json.Unmarshal([]byte(s.EnvJSON), &env)

	if env == nil {
		env = map[string]string{}
	}

	var headers map[string]string
	_ = json.Unmarshal([]byte(s.HeadersJSON), &headers)
	if headers == nil {
		headers = map[string]string{}
	}

	ow := int32(0)
	if s.UserID != nil {
		ow = int32(*s.UserID)
	}

	return &runnerpb.MCPServer{
		Id:             s.ID,
		Name:           s.Name,
		Enabled:        s.Enabled,
		Transport:      s.Transport,
		Command:        s.Command,
		Args:           args,
		Env:            maskSecretMap(env),
		Url:            s.URL,
		Headers:        maskSecretMap(headers),
		TimeoutSeconds: s.TimeoutSeconds,
		OwnerUserId:    ow,
	}
}

func domainMCPServerFromCreate(req *runnerpb.CreateMCPServerRequest) (*domain.MCPServer, error) {
	if req == nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_EMPTY_REQUEST", "пустой запрос")
	}

	argsB, err := json.Marshal(req.GetArgs())
	if err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_ARGS_JSON_INVALID", "args: "+err.Error())
	}

	env := req.GetEnv()
	if env == nil {
		env = map[string]string{}
	}

	envB, err := json.Marshal(env)
	if err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_ENV_JSON_INVALID", "env: "+err.Error())
	}

	headers := req.GetHeaders()
	if headers == nil {
		headers = map[string]string{}
	}

	headersB, err := json.Marshal(headers)
	if err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_HEADERS_JSON_INVALID", "headers: "+err.Error())
	}

	return &domain.MCPServer{
		Name:           req.GetName(),
		Enabled:        req.GetEnabled(),
		Transport:      strings.TrimSpace(req.GetTransport()),
		Command:        strings.TrimSpace(req.GetCommand()),
		ArgsJSON:       string(argsB),
		EnvJSON:        string(envB),
		URL:            strings.TrimSpace(req.GetUrl()),
		HeadersJSON:    string(headersB),
		TimeoutSeconds: req.GetTimeoutSeconds(),
	}, nil
}

func (h *RunnerHandler) ListMCPServers(ctx context.Context, _ *commonpb.Empty) (*runnerpb.ListMCPServersResponse, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}

	if h.mcpServersUC == nil {
		return nil, StatusErrorWithReason(codes.Internal, "MCP_SERVICE_UNAVAILABLE", "MCP недоступен")
	}

	list, err := h.mcpServersUC.ListGlobal(ctx)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	out := make([]*runnerpb.MCPServer, 0, len(list))
	for _, s := range list {
		out = append(out, mapDomainMCPServerToProto(s))
	}

	return &runnerpb.ListMCPServersResponse{Servers: out}, nil
}

func (h *RunnerHandler) CreateMCPServer(ctx context.Context, req *runnerpb.CreateMCPServerRequest) (*runnerpb.MCPServer, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}

	if h.mcpServersUC == nil {
		return nil, StatusErrorWithReason(codes.Internal, "MCP_SERVICE_UNAVAILABLE", "MCP недоступен")
	}

	d, err := domainMCPServerFromCreate(req)
	if err != nil {
		return nil, err
	}

	if err := h.validateMCPServerFull(d); err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_VALIDATION_INVALID_ARGUMENT", err.Error())
	}

	s, err := h.mcpServersUC.CreateGlobal(ctx, d)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	h.mcpToolsListCache.InvalidateServerID(s.ID)

	if u, err := GetUserFromContext(ctx, h.authUseCase); err == nil && u != nil {
		logMCPServerStdioAudit("create", s, u.Id)
	} else {
		logMCPServerStdioAudit("create", s, 0)
	}

	return mapDomainMCPServerToProto(s), nil
}

func (h *RunnerHandler) UpdateMCPServer(ctx context.Context, req *runnerpb.UpdateMCPServerRequest) (*runnerpb.MCPServer, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}

	if h.mcpServersUC == nil {
		return nil, StatusErrorWithReason(codes.Internal, "MCP_SERVICE_UNAVAILABLE", "MCP недоступен")
	}

	if req == nil || req.GetId() <= 0 {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}

	prev, err := h.mcpServersUC.GetGlobal(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "MCP_SERVER_NOT_FOUND", "MCP-сервер не найден")
		}
		return nil, ToStatusError(codes.Internal, err)
	}

	var prevEnv, prevHdr map[string]string
	_ = json.Unmarshal([]byte(prev.EnvJSON), &prevEnv)
	_ = json.Unmarshal([]byte(prev.HeadersJSON), &prevHdr)
	argsB, err := json.Marshal(req.GetArgs())
	if err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_ARGS_JSON_INVALID", "args: "+err.Error())
	}

	env := req.GetEnv()
	if env == nil {
		env = map[string]string{}
	}

	env = mergeMaskedSecretMaps(env, prevEnv)
	envB, err := json.Marshal(env)
	if err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_ENV_JSON_INVALID", "env: "+err.Error())
	}

	headers := req.GetHeaders()
	if headers == nil {
		headers = map[string]string{}
	}

	headers = mergeMaskedSecretMaps(headers, prevHdr)
	headersB, err := json.Marshal(headers)
	if err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_HEADERS_JSON_INVALID", "headers: "+err.Error())
	}

	d := &domain.MCPServer{
		ID:             req.GetId(),
		Name:           req.GetName(),
		Enabled:        req.GetEnabled(),
		Transport:      strings.TrimSpace(req.GetTransport()),
		Command:        strings.TrimSpace(req.GetCommand()),
		ArgsJSON:       string(argsB),
		EnvJSON:        string(envB),
		URL:            strings.TrimSpace(req.GetUrl()),
		HeadersJSON:    string(headersB),
		TimeoutSeconds: req.GetTimeoutSeconds(),
	}

	if err := h.validateMCPServerFull(d); err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_VALIDATION_INVALID_ARGUMENT", err.Error())
	}

	if err := h.mcpServersUC.UpdateGlobal(ctx, d); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "MCP_SERVER_NOT_FOUND", "MCP-сервер не найден")
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	s, err := h.mcpServersUC.GetGlobal(ctx, req.GetId())
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	h.mcpToolsListCache.InvalidateServerID(req.GetId())

	if u, err := GetUserFromContext(ctx, h.authUseCase); err == nil && u != nil {
		logMCPServerStdioAudit("update", s, u.Id)
	} else {
		logMCPServerStdioAudit("update", s, 0)
	}

	return mapDomainMCPServerToProto(s), nil
}

func (h *RunnerHandler) DeleteMCPServer(ctx context.Context, req *runnerpb.DeleteMCPServerRequest) (*commonpb.Empty, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}

	if h.mcpServersUC == nil {
		return nil, StatusErrorWithReason(codes.Internal, "MCP_SERVICE_UNAVAILABLE", "MCP недоступен")
	}

	if req == nil || req.GetId() <= 0 {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}

	id := req.GetId()
	if err := h.mcpServersUC.DeleteGlobal(ctx, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "MCP_SERVER_NOT_FOUND", "MCP-сервер не найден")
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	h.mcpToolsListCache.InvalidateServerID(id)

	return &commonpb.Empty{}, nil
}

func (h *RunnerHandler) ListUserMCPServers(ctx context.Context, _ *commonpb.Empty) (*runnerpb.ListMCPServersResponse, error) {
	user, err := GetUserFromContext(ctx, h.authUseCase)
	if err != nil {
		return nil, err
	}

	if h.mcpServersUC == nil {
		return nil, StatusErrorWithReason(codes.Internal, "MCP_SERVICE_UNAVAILABLE", "MCP недоступен")
	}

	list, err := h.mcpServersUC.ListForUser(ctx, user.Id)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	out := make([]*runnerpb.MCPServer, 0, len(list))
	for _, s := range list {
		out = append(out, mapDomainMCPServerToProto(s))
	}

	return &runnerpb.ListMCPServersResponse{Servers: out}, nil
}

func (h *RunnerHandler) CreateUserMCPServer(ctx context.Context, req *runnerpb.CreateMCPServerRequest) (*runnerpb.MCPServer, error) {
	user, err := GetUserFromContext(ctx, h.authUseCase)
	if err != nil {
		return nil, err
	}

	if h.mcpServersUC == nil {
		return nil, StatusErrorWithReason(codes.Internal, "MCP_SERVICE_UNAVAILABLE", "MCP недоступен")
	}

	d, err := domainMCPServerFromCreate(req)
	if err != nil {
		return nil, err
	}

	if err := h.validateMCPServerFull(d); err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_VALIDATION_INVALID_ARGUMENT", err.Error())
	}

	if max := h.cfg.MCP.MaxMCPServersPerUser; max > 0 {
		n, err := h.mcpServersUC.CountOwnedByUser(ctx, user.Id)
		if err != nil {
			return nil, ToStatusError(codes.Internal, err)
		}

		if n >= int64(max) {
			return nil, StatusErrorWithReason(codes.ResourceExhausted, "MCP_USER_SERVER_LIMIT_EXCEEDED",
				fmt.Sprintf("достигнут лимит личных MCP-серверов (%d); удалите сервер или увеличьте mcp.max_mcp_servers_per_user", max))
		}
	}

	s, err := h.mcpServersUC.CreateOwned(ctx, d, user.Id)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	h.mcpToolsListCache.InvalidateServerID(s.ID)
	logMCPServerStdioAudit("create", s, user.Id)

	return mapDomainMCPServerToProto(s), nil
}

func (h *RunnerHandler) UpdateUserMCPServer(ctx context.Context, req *runnerpb.UpdateMCPServerRequest) (*runnerpb.MCPServer, error) {
	user, err := GetUserFromContext(ctx, h.authUseCase)
	if err != nil {
		return nil, err
	}

	if h.mcpServersUC == nil {
		return nil, StatusErrorWithReason(codes.Internal, "MCP_SERVICE_UNAVAILABLE", "MCP недоступен")
	}

	if req == nil || req.GetId() <= 0 {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}

	prev, err := h.mcpServersUC.GetForUser(ctx, req.GetId(), user.Id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "MCP_SERVER_NOT_FOUND", "MCP-сервер не найден")
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	if prev.UserID == nil || *prev.UserID != user.Id {
		return nil, StatusErrorWithReason(codes.PermissionDenied, "MCP_EDIT_OWNED_ONLY", "можно редактировать только свои MCP-серверы")
	}

	var prevEnv, prevHdr map[string]string
	_ = json.Unmarshal([]byte(prev.EnvJSON), &prevEnv)
	_ = json.Unmarshal([]byte(prev.HeadersJSON), &prevHdr)
	argsB, err := json.Marshal(req.GetArgs())
	if err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_ARGS_JSON_INVALID", "args: "+err.Error())
	}

	env := req.GetEnv()
	if env == nil {
		env = map[string]string{}
	}

	env = mergeMaskedSecretMaps(env, prevEnv)
	envB, err := json.Marshal(env)
	if err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_ENV_JSON_INVALID", "env: "+err.Error())
	}

	headers := req.GetHeaders()
	if headers == nil {
		headers = map[string]string{}
	}

	headers = mergeMaskedSecretMaps(headers, prevHdr)
	headersB, err := json.Marshal(headers)
	if err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_HEADERS_JSON_INVALID", "headers: "+err.Error())
	}

	d := &domain.MCPServer{
		ID:             req.GetId(),
		Name:           req.GetName(),
		Enabled:        req.GetEnabled(),
		Transport:      strings.TrimSpace(req.GetTransport()),
		Command:        strings.TrimSpace(req.GetCommand()),
		ArgsJSON:       string(argsB),
		EnvJSON:        string(envB),
		URL:            strings.TrimSpace(req.GetUrl()),
		HeadersJSON:    string(headersB),
		TimeoutSeconds: req.GetTimeoutSeconds(),
	}

	if err := h.validateMCPServerFull(d); err != nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "MCP_VALIDATION_INVALID_ARGUMENT", err.Error())
	}

	if err := h.mcpServersUC.UpdateOwned(ctx, d, user.Id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "MCP_SERVER_NOT_FOUND", "MCP-сервер не найден")
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	s, err := h.mcpServersUC.GetForUser(ctx, req.GetId(), user.Id)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	h.mcpToolsListCache.InvalidateServerID(req.GetId())
	logMCPServerStdioAudit("update", s, user.Id)

	return mapDomainMCPServerToProto(s), nil
}

func (h *RunnerHandler) DeleteUserMCPServer(ctx context.Context, req *runnerpb.DeleteMCPServerRequest) (*commonpb.Empty, error) {
	user, err := GetUserFromContext(ctx, h.authUseCase)
	if err != nil {
		return nil, err
	}

	if h.mcpServersUC == nil {
		return nil, StatusErrorWithReason(codes.Internal, "MCP_SERVICE_UNAVAILABLE", "MCP недоступен")
	}

	if req == nil || req.GetId() <= 0 {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}

	prev, err := h.mcpServersUC.GetForUser(ctx, req.GetId(), user.Id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "MCP_SERVER_NOT_FOUND", "MCP-сервер не найден")
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	if prev.UserID == nil || *prev.UserID != user.Id {
		return nil, StatusErrorWithReason(codes.PermissionDenied, "MCP_DELETE_OWNED_ONLY", "можно удалять только свои MCP-серверы")
	}

	id := req.GetId()
	if err := h.mcpServersUC.DeleteOwned(ctx, id, user.Id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "MCP_SERVER_NOT_FOUND", "MCP-сервер не найден")
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	h.mcpToolsListCache.InvalidateServerID(id)

	return &commonpb.Empty{}, nil
}

func (h *RunnerHandler) probeMCPServerToResult(ctx context.Context, s *domain.MCPServer) *runnerpb.MCPProbeResult {
	if !s.Enabled {
		return &runnerpb.MCPProbeResult{Ok: false, ErrorMessage: "сервер отключён"}
	}
	d := *s
	if err := h.validateMCPServerFull(&d); err != nil {
		return &runnerpb.MCPProbeResult{Ok: false, ErrorMessage: err.Error()}
	}
	pr, err := mcpclient.ProbeServer(ctx, &d, h.mcpToolsListCache)
	if err != nil {
		return &runnerpb.MCPProbeResult{Ok: false, ErrorMessage: err.Error()}
	}
	return &runnerpb.MCPProbeResult{
		Ok:              true,
		ProtocolVersion: pr.ProtocolVersion,
		ServerName:      pr.ServerName,
		ServerVersion:   pr.ServerVersion,
		Instructions:    pr.Instructions,
		HasTools:        pr.HasTools,
		HasResources:    pr.HasResources,
		HasPrompts:      pr.HasPrompts,
	}
}

func (h *RunnerHandler) ProbeUserMCPServer(ctx context.Context, req *runnerpb.GetMCPServerRequest) (*runnerpb.MCPProbeResult, error) {
	user, err := GetUserFromContext(ctx, h.authUseCase)
	if err != nil {
		return nil, err
	}

	if h.mcpServersUC == nil {
		return nil, StatusErrorWithReason(codes.Internal, "MCP_SERVICE_UNAVAILABLE", "MCP недоступен")
	}

	if req == nil || req.GetId() <= 0 {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}

	s, err := h.mcpServersUC.GetForUser(ctx, req.GetId(), user.Id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "MCP_SERVER_NOT_FOUND", "MCP-сервер не найден")
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	return h.probeMCPServerToResult(ctx, s), nil
}

func (h *RunnerHandler) ProbeMCPServer(ctx context.Context, req *runnerpb.GetMCPServerRequest) (*runnerpb.MCPProbeResult, error) {
	if err := RequireAdmin(ctx, h.authUseCase); err != nil {
		return nil, err
	}

	if h.mcpServersUC == nil {
		return nil, StatusErrorWithReason(codes.Internal, "MCP_SERVICE_UNAVAILABLE", "MCP недоступен")
	}

	if req == nil || req.GetId() <= 0 {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "RUNNER_POSITIVE_ID_REQUIRED", "нужен положительный id")
	}

	s, err := h.mcpServersUC.GetGlobal(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "MCP_SERVER_NOT_FOUND", "MCP-сервер не найден")
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	return h.probeMCPServerToResult(ctx, s), nil
}
