package runner

import (
	"context"
	"strings"
	"time"

	"github.com/magomedcoder/llm-runner/domain"
	"github.com/magomedcoder/llm-runner/gpu"
	"github.com/magomedcoder/llm-runner/pb"
	"github.com/magomedcoder/llm-runner/provider"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	pb.UnimplementedLLMRunnerServiceServer
	textProvider     provider.TextProvider
	gpuCollector     gpu.Collector
	inferenceMetrics *InferenceMetrics
	sem              chan struct{}
	defaultModel     string
}

func NewServer(textProvider provider.TextProvider, gpuCollector gpu.Collector, maxConcurrentGenerations int, defaultModel string) *Server {
	if gpuCollector == nil {
		gpuCollector = gpu.NewCollector()
	}
	var sem chan struct{}
	if maxConcurrentGenerations > 0 {
		sem = make(chan struct{}, maxConcurrentGenerations)
	}
	return &Server{
		textProvider:     textProvider,
		gpuCollector:     gpuCollector,
		inferenceMetrics: NewInferenceMetrics(),
		sem:              sem,
		defaultModel:     strings.TrimSpace(defaultModel),
	}
}

func (s *Server) CheckConnection(ctx context.Context, _ *pb.Empty) (*pb.ConnectionResponse, error) {
	if s.textProvider == nil {
		return &pb.ConnectionResponse{IsConnected: false}, nil
	}

	ok, _ := s.textProvider.CheckConnection(ctx)
	return &pb.ConnectionResponse{IsConnected: ok}, nil
}

func (s *Server) GetModels(ctx context.Context, _ *pb.Empty) (*pb.GetModelsResponse, error) {
	if s.textProvider == nil {
		return &pb.GetModelsResponse{}, nil
	}

	models, err := s.textProvider.GetModels(ctx)
	if err != nil {
		return &pb.GetModelsResponse{}, nil
	}

	return &pb.GetModelsResponse{
		Models: models,
	}, nil
}

func (s *Server) SendMessage(req *pb.SendMessageRequest, stream pb.LLMRunnerService_SendMessageServer) error {
	if s.textProvider == nil {
		return status.Error(codes.Unavailable, "текстовый провайдер не подключён")
	}

	if req == nil || len(req.Messages) == 0 {
		return stream.Send(&pb.ChatResponse{Done: true})
	}

	ctx := stream.Context()

	if s.sem != nil {
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	sessionID := req.SessionId
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = s.defaultModel
	}
	messages := domain.AIMessagesFromProto(req.Messages, sessionID)
	stopSequences := req.GetStopSequences()

	if ts := req.GetTimeoutSeconds(); ts > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(ts)*time.Second)
		defer cancel()
	}

	start := time.Now()
	var tokens int64

	ch, err := s.textProvider.SendMessage(ctx, sessionID, model, messages, stopSequences, nil)
	if err != nil {
		_ = stream.Send(&pb.ChatResponse{Done: true})
		return err
	}

	for chunk := range ch {
		if chunk != "" {
			tokens++
			if err := stream.Send(&pb.ChatResponse{
				Content: chunk,
				Done:    false,
			}); err != nil {
				return err
			}
		}
	}

	if s.inferenceMetrics != nil {
		s.inferenceMetrics.Record(tokens, time.Since(start))
	}

	return stream.Send(&pb.ChatResponse{Done: true})
}

func (s *Server) GetGpuInfo(ctx context.Context, _ *pb.Empty) (*pb.GetGpuInfoResponse, error) {
	list := s.gpuCollector.Collect()
	gpus := make([]*pb.GpuInfo, len(list))
	for i := range list {
		gpus[i] = &pb.GpuInfo{
			Name:               list[i].Name,
			TemperatureC:       list[i].TemperatureC,
			MemoryTotalMb:      list[i].MemoryTotalMB,
			MemoryUsedMb:       list[i].MemoryUsedMB,
			UtilizationPercent: list[i].UtilizationPercent,
		}
	}

	return &pb.GetGpuInfoResponse{Gpus: gpus}, nil
}

func (s *Server) GetServerInfo(ctx context.Context, _ *pb.Empty) (*pb.ServerInfo, error) {
	si := CollectSysInfo()
	out := &pb.ServerInfo{
		Hostname:      si.Hostname,
		Os:            si.OS,
		Arch:          si.Arch,
		CpuCores:      si.CPUCores,
		MemoryTotalMb: si.MemoryTotalMB,
	}
	if s.textProvider != nil {
		if models, err := s.textProvider.GetModels(ctx); err == nil && len(models) > 0 {
			out.Models = models
		}
	}

	return out, nil
}
