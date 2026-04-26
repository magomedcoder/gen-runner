package llm_runner

import (
	"context"
	"github.com/magomedcoder/gen/api/pb/llm-runner/llmrunnerpb"
	"github.com/magomedcoder/gen/llm-runner/domain"
	"github.com/magomedcoder/gen/llm-runner/gpu"
	"github.com/magomedcoder/gen/llm-runner/provider"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"time"
)

func mapGenerationParamsFromProto(in *llmrunnerpb.GenerationParams) *domain.GenerationParams {
	if in == nil {
		return nil
	}

	out := &domain.GenerationParams{}
	if in.Temperature != nil {
		v := in.GetTemperature()
		out.Temperature = &v
	}

	if in.MaxTokens != nil {
		v := in.GetMaxTokens()
		out.MaxTokens = &v
	}

	if in.TopK != nil {
		v := in.GetTopK()
		out.TopK = &v
	}

	if in.TopP != nil {
		v := in.GetTopP()
		out.TopP = &v
	}

	if in.EnableThinking != nil {
		v := in.GetEnableThinking()
		out.EnableThinking = &v
	}

	if rf := in.GetResponseFormat(); rf != nil {
		var schema *string
		if rf.Schema != nil {
			v := rf.GetSchema()
			schema = &v
		}

		out.ResponseFormat = &domain.ResponseFormat{
			Type:   rf.GetType(),
			Schema: schema,
		}
	}

	if len(in.GetTools()) > 0 {
		out.Tools = make([]domain.Tool, 0, len(in.GetTools()))
		for _, t := range in.GetTools() {
			if t == nil {
				continue
			}

			out.Tools = append(out.Tools, domain.Tool{
				Name:           t.GetName(),
				Description:    t.GetDescription(),
				ParametersJSON: t.GetParametersJson(),
			})
		}
	}

	return out
}

type Server struct {
	llmrunnerpb.UnimplementedLLMRunnerServiceServer
	textProvider     provider.TextProvider
	gpuCollector     gpu.Collector
	inferenceMetrics *InferenceMetrics
	sem              chan struct{}
	unloadAfterRPC   bool
	modelsDir        string
}

func NewServer(textProvider provider.TextProvider, gpuCollector gpu.Collector, maxConcurrentGenerations int, unloadAfterRPC bool, modelsDir string) *Server {
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
		unloadAfterRPC:   unloadAfterRPC,
		modelsDir:        strings.TrimSpace(modelsDir),
	}
}

func (s *Server) maybeUnloadAfterRPC() {
	if !s.unloadAfterRPC || s.textProvider == nil {
		return
	}

	_ = s.textProvider.UnloadModel(context.Background())
}

func (s *Server) CheckConnection(ctx context.Context, _ *llmrunnerpb.Empty) (*llmrunnerpb.ConnectionResponse, error) {
	if s.textProvider == nil {
		return &llmrunnerpb.ConnectionResponse{IsConnected: false}, nil
	}

	ok, _ := s.textProvider.CheckConnection(ctx)
	return &llmrunnerpb.ConnectionResponse{IsConnected: ok}, nil
}

func (s *Server) RunnerProbe(ctx context.Context, _ *llmrunnerpb.Empty) (*llmrunnerpb.RunnerProbeResponse, error) {
	out := &llmrunnerpb.RunnerProbeResponse{}
	if s.textProvider != nil {
		ok, _ := s.textProvider.CheckConnection(ctx)
		out.BackendConnected = ok
	}

	si, _ := s.GetServerInfo(ctx, &llmrunnerpb.Empty{})
	out.Server = si
	gi, _ := s.GetGpuInfo(ctx, &llmrunnerpb.Empty{})
	if gi != nil {
		out.Gpus = gi.Gpus
	}

	lm, err := s.GetLoadedModel(ctx, &llmrunnerpb.Empty{})
	if err != nil {
		lm = &llmrunnerpb.GetLoadedModelResponse{}
	}

	out.LoadedModel = lm

	return out, nil
}

func (s *Server) GetModels(ctx context.Context, _ *llmrunnerpb.Empty) (*llmrunnerpb.GetModelsResponse, error) {
	if s.textProvider == nil {
		return &llmrunnerpb.GetModelsResponse{}, nil
	}

	models, err := s.textProvider.GetModels(ctx)
	if err != nil {
		return &llmrunnerpb.GetModelsResponse{}, nil
	}

	return &llmrunnerpb.GetModelsResponse{
		Models: models,
	}, nil
}

func (s *Server) SendMessage(req *llmrunnerpb.SendMessageRequest, stream llmrunnerpb.LLMRunnerService_SendMessageServer) error {
	if s.textProvider == nil {
		return status.Error(codes.Unavailable, "текстовый провайдер не подключён")
	}

	if req == nil || len(req.Messages) == 0 {
		return stream.Send(&llmrunnerpb.ChatResponse{Done: true})
	}

	ctx := stream.Context()
	traceID := incomingTraceID(ctx)

	semInUse, semCap := 0, 0
	if s.sem != nil {
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
			semInUse = len(s.sem)
			semCap = cap(s.sem)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	sessionID := req.SessionId
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return status.Error(codes.InvalidArgument, "укажите model в запросе или default_model в config.yaml")
	}

	logger.V("SendMessage: session_id=%d trace_id=%q model=%q sem_in_use=%d/%d", sessionID, traceID, model, semInUse, semCap)
	messages := domain.AIMessagesFromProto(req.Messages, sessionID)
	stopSequences := req.GetStopSequences()
	genParams := mapGenerationParamsFromProto(req.GetGenerationParams())

	if ts := req.GetTimeoutSeconds(); ts > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(ts)*time.Second)
		defer cancel()
	}

	start := time.Now()
	var tokens int64
	var ttft time.Duration
	firstToken := false

	ch, err := s.textProvider.SendMessage(ctx, sessionID, model, messages, stopSequences, genParams)
	if err != nil {
		s.maybeUnloadAfterRPC()
		return status.Errorf(codes.Internal, "ошибка генерации: %v", err)
	}

	hasContent := false
	var fullOutput strings.Builder
	for chunk := range ch {
		if chunk.Content == "" && chunk.ReasoningContent == "" {
			continue
		}
		if chunk.Content != "" {
			fullOutput.WriteString(chunk.Content)
		}
		if !firstToken {
			ttft = time.Since(start)
			firstToken = true
		}

		hasContent = true
		tokens++
		resp := &llmrunnerpb.ChatResponse{Done: false, Content: chunk.Content}
		if rc := chunk.ReasoningContent; rc != "" {
			resp.ReasoningContent = &rc
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}

	wall := time.Since(start)
	if s.inferenceMetrics != nil {
		s.inferenceMetrics.Record(tokens, wall, ttft)
	}

	if hasContent {
		_, _, tps, ttftMs := s.inferenceMetrics.Get()
		logger.V("inference session_id=%d trace_id=%q tokens=%d wall_ms=%.1f ttft_ms=%.1f tps=%.1f sem=%d/%d", sessionID, traceID, tokens, wall.Seconds()*1000, ttftMs, tps, semInUse, semCap)
	}

	if !hasContent {
		s.maybeUnloadAfterRPC()
		return status.Error(codes.Internal, "модель не вернула ответ")
	}

	if genParams != nil && len(genParams.Tools) > 0 {
		if blob := ExtractToolActionBlob(strings.TrimSpace(fullOutput.String())); blob != "" {
			b := blob
			if err := stream.Send(&llmrunnerpb.ChatResponse{ToolActionJson: &b, Done: false}); err != nil {
				return err
			}
		}
	}

	if err := stream.Send(&llmrunnerpb.ChatResponse{Done: true}); err != nil {
		return err
	}
	s.maybeUnloadAfterRPC()

	return nil
}

func (s *Server) Embed(ctx context.Context, req *llmrunnerpb.EmbedRequest) (*llmrunnerpb.EmbedResponse, error) {
	if s.textProvider == nil {
		return nil, status.Error(codes.Unavailable, "текстовый провайдер не подключён")
	}

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}

	text := strings.TrimSpace(req.GetText())
	if text == "" {
		return nil, status.Error(codes.InvalidArgument, "text не может быть пустым")
	}

	model := strings.TrimSpace(req.GetModel())
	if model == "" {
		return nil, status.Error(codes.InvalidArgument, "укажите model в запросе или default_model в config.yaml")
	}

	logger.V("Embed: trace_id=%q model=%q", incomingTraceID(ctx), model)

	if s.sem != nil {
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	defer s.maybeUnloadAfterRPC()

	vec, err := s.textProvider.Embed(ctx, model, text)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "эмбеддинг: %v", err)
	}

	return &llmrunnerpb.EmbedResponse{Values: vec}, nil
}

func (s *Server) EmbedBatch(ctx context.Context, req *llmrunnerpb.EmbedBatchRequest) (*llmrunnerpb.EmbedBatchResponse, error) {
	if s.textProvider == nil {
		return nil, status.Error(codes.Unavailable, "текстовый провайдер не подключён")
	}

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}

	texts := req.GetTexts()
	if len(texts) == 0 {
		return nil, status.Error(codes.InvalidArgument, "texts не может быть пустым")
	}

	for i, t := range texts {
		if strings.TrimSpace(t) == "" {
			return nil, status.Errorf(codes.InvalidArgument, "texts[%d]: пустая строка", i)
		}
	}

	model := strings.TrimSpace(req.GetModel())
	if model == "" {
		return nil, status.Error(codes.InvalidArgument, "укажите model в запросе или default_model в config.yaml")
	}

	logger.V("EmbedBatch: trace_id=%q model=%q n=%d", incomingTraceID(ctx), model, len(texts))

	defer s.maybeUnloadAfterRPC()

	if s.sem != nil {
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	vecs, err := s.textProvider.EmbedBatch(ctx, model, texts)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "эмбеддинг batch: %v", err)
	}
	if len(vecs) != len(texts) {
		return nil, status.Errorf(codes.Internal, "эмбеддинг batch: несоответствие размеров (%d != %d)", len(vecs), len(texts))
	}

	out := &llmrunnerpb.EmbedBatchResponse{
		Embeddings: make([]*llmrunnerpb.Embedding, 0, len(vecs)),
	}
	for _, vec := range vecs {
		out.Embeddings = append(out.Embeddings, &llmrunnerpb.Embedding{Values: vec})
	}

	return out, nil
}

func (s *Server) GetGpuInfo(ctx context.Context, _ *llmrunnerpb.Empty) (*llmrunnerpb.GetGpuInfoResponse, error) {
	list := s.gpuCollector.Collect()
	gpus := make([]*llmrunnerpb.GpuInfo, len(list))
	for i := range list {
		gpus[i] = &llmrunnerpb.GpuInfo{
			Name:               list[i].Name,
			TemperatureC:       list[i].TemperatureC,
			MemoryTotalMb:      list[i].MemoryTotalMB,
			MemoryUsedMb:       list[i].MemoryUsedMB,
			UtilizationPercent: list[i].UtilizationPercent,
		}
	}

	return &llmrunnerpb.GetGpuInfoResponse{Gpus: gpus}, nil
}

func (s *Server) GetServerInfo(ctx context.Context, _ *llmrunnerpb.Empty) (*llmrunnerpb.ServerInfo, error) {
	si := CollectSysInfo()
	out := &llmrunnerpb.ServerInfo{
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

func (s *Server) GetLoadedModel(ctx context.Context, _ *llmrunnerpb.Empty) (*llmrunnerpb.GetLoadedModelResponse, error) {
	if s.textProvider == nil {
		return &llmrunnerpb.GetLoadedModelResponse{}, nil
	}

	loaded, base, disp, err := s.textProvider.GetLoadedModel(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetLoadedModel: %v", err)
	}

	return &llmrunnerpb.GetLoadedModelResponse{
		Loaded:       loaded,
		GgufBasename: base,
		DisplayName:  disp,
	}, nil
}

func (s *Server) unloadProviderModel(ctx context.Context, op string) (*llmrunnerpb.Empty, error) {
	if s.textProvider == nil {
		return &llmrunnerpb.Empty{}, nil
	}

	if err := s.textProvider.UnloadModel(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "%s: %v", op, err)
	}

	return &llmrunnerpb.Empty{}, nil
}

func (s *Server) UnloadModel(ctx context.Context, _ *llmrunnerpb.Empty) (*llmrunnerpb.Empty, error) {
	return s.unloadProviderModel(ctx, "UnloadModel")
}

// ResetMemory полностью выгружает текущую модель и освобождает VRAM/RAM процесса (тот же эффект, что UnloadModel).
func (s *Server) ResetMemory(ctx context.Context, _ *llmrunnerpb.Empty) (*llmrunnerpb.Empty, error) {
	return s.unloadProviderModel(ctx, "ResetMemory")
}
