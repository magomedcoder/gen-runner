package handler

import (
	"context"

	"github.com/magomedcoder/gen/api/pb/runnerpb"
	"github.com/magomedcoder/gen/internal/runner"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/logger"
)

type RunnerHandler struct {
	runnerpb.UnimplementedRunnerServiceServer
	runnerpb.UnimplementedRunnerAdminServiceServer
	registry    *runner.Registry
	pool        *runner.Pool
	authUseCase *usecase.AuthUseCase
}

func NewRunnerHandler(registry *runner.Registry, pool *runner.Pool, authUseCase *usecase.AuthUseCase) *RunnerHandler {
	return &RunnerHandler{
		registry:    registry,
		pool:        pool,
		authUseCase: authUseCase,
	}
}

func (h *RunnerHandler) GetRunners(ctx context.Context, _ *runnerpb.Empty) (*runnerpb.GetRunnersResponse, error) {
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

		conn, gpuList, si := h.pool.ProbeLLMRunner(ctx, r.Address)
		r.Connected = conn
		if len(gpuList) > 0 {
			r.Gpus = gpuList
		}

		if si != nil {
			r.ServerInfo = si
		}
	}
	logger.V("GetRunners: возвращено раннеров: %d", len(runners))
	return &runnerpb.GetRunnersResponse{
		Runners: runners,
	}, nil
}

func (h *RunnerHandler) SetRunnerEnabled(ctx context.Context, req *runnerpb.SetRunnerEnabledRequest) (*runnerpb.Empty, error) {
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

	return &runnerpb.Empty{}, nil
}

func (h *RunnerHandler) GetRunnersStatus(ctx context.Context, _ *runnerpb.Empty) (*runnerpb.GetRunnersStatusResponse, error) {
	return &runnerpb.GetRunnersStatusResponse{
		HasActiveRunners: h.registry.HasActiveRunners(),
	}, nil
}
