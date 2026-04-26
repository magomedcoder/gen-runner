package provider

import (
	"context"
	"fmt"
	"github.com/magomedcoder/gen/llm-runner/config"
	"strings"

	"github.com/magomedcoder/gen/llm-runner/domain"
	"github.com/magomedcoder/gen/llm-runner/service"
)

type TextBackend interface {
	CheckConnection(ctx context.Context) (bool, error)

	WarmDefaultModel(ctx context.Context, model string) error

	GetModels(ctx context.Context) ([]string, error)

	GetLoadedModel(ctx context.Context) (loaded bool, ggufBasename, displayName string, err error)

	UnloadModel(ctx context.Context) error

	SendMessage(ctx context.Context, model string, messages []*domain.AIChatMessage, stopSequences []string, genParams *domain.GenerationParams) (chan domain.TextStreamChunk, error)

	Embed(ctx context.Context, model string, text string) ([]float32, error)

	EmbedBatch(ctx context.Context, model string, texts []string) ([][]float32, error)
}

type TextProvider interface {
	CheckConnection(ctx context.Context) (bool, error)

	WarmDefaultModel(ctx context.Context, model string) error

	GetModels(ctx context.Context) ([]string, error)

	GetLoadedModel(ctx context.Context) (loaded bool, ggufBasename, displayName string, err error)

	UnloadModel(ctx context.Context) error

	SendMessage(ctx context.Context, sessionId int64, model string, messages []*domain.AIChatMessage, stopSequences []string, genParams *domain.GenerationParams) (chan domain.TextStreamChunk, error)

	Embed(ctx context.Context, model string, text string) ([]float32, error)

	EmbedBatch(ctx context.Context, model string, texts []string) ([][]float32, error)
}

func NewTextProvider(cfg *config.Config) (TextProvider, error) {
	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("укажите model_path")
	}

	var opts []service.LlamaOption
	nCtx := cfg.MaxContextTokens
	if nCtx <= 0 {
		nCtx = 4096
	}

	opts = append(opts, service.WithLlamaNCtx(nCtx))
	if cfg.MaxContextTokens > 0 {
		opts = append(opts, service.WithMaxContextTokens(cfg.MaxContextTokens))
	}

	opts = append(opts, service.WithEmbeddings(true))
	if cfg.GpuLayers != 0 {
		opts = append(opts, service.WithGPULayers(cfg.GpuLayers))
	}

	if cfg.MLock {
		opts = append(opts, service.WithMLock(true))
	}

	if cfg.MMap != nil {
		opts = append(opts, service.WithMMap(*cfg.MMap))
	}

	if strings.TrimSpace(cfg.MainGPU) != "" {
		opts = append(opts, service.WithMainGPU(cfg.MainGPU))
	}

	if strings.TrimSpace(cfg.TensorSplit) != "" {
		opts = append(opts, service.WithTensorSplit(cfg.TensorSplit))
	}

	if cfg.SilentLoading {
		opts = append(opts, service.WithSilentLoading(true))
	}

	if cfg.Threads > 0 {
		opts = append(opts, service.WithThreads(cfg.Threads))
	}

	if cfg.ThreadsBatch > 0 {
		opts = append(opts, service.WithThreadsBatch(cfg.ThreadsBatch))
	}

	if cfg.BatchSize > 0 {
		opts = append(opts, service.WithBatchSize(cfg.BatchSize))
	}

	if cfg.F16Memory {
		opts = append(opts, service.WithF16Memory(true))
	}

	if strings.TrimSpace(cfg.KVCacheType) != "" {
		opts = append(opts, service.WithKVCacheType(cfg.KVCacheType))
	}

	if strings.TrimSpace(cfg.FlashAttn) != "" {
		opts = append(opts, service.WithFlashAttn(cfg.FlashAttn))
	}

	if cfg.PrefixCaching != nil {
		opts = append(opts, service.WithPrefixCaching(*cfg.PrefixCaching))
	}

	if cfg.Parallel > 0 {
		opts = append(opts, service.WithParallel(cfg.Parallel))
	}

	if cfg.TopNSigma != nil {
		opts = append(opts, service.WithTopNSigma(*cfg.TopNSigma))
	}

	if cfg.FrequencyPenalty != nil {
		opts = append(opts, service.WithFrequencyPenalty(*cfg.FrequencyPenalty))
	}

	if cfg.PresencePenalty != nil {
		opts = append(opts, service.WithPresencePenalty(*cfg.PresencePenalty))
	}

	if cfg.IgnoreEOS != nil {
		opts = append(opts, service.WithIgnoreEOS(*cfg.IgnoreEOS))
	}

	if cfg.DRYMultiplier != nil {
		opts = append(opts, service.WithDRYMultiplier(*cfg.DRYMultiplier))
	}

	if cfg.DRYBase != nil {
		opts = append(opts, service.WithDRYBase(*cfg.DRYBase))
	}

	if cfg.DRYAllowedLength != nil {
		opts = append(opts, service.WithDRYAllowedLength(*cfg.DRYAllowedLength))
	}

	if cfg.DRYPenaltyLastN != nil {
		opts = append(opts, service.WithDRYPenaltyLastN(*cfg.DRYPenaltyLastN))
	}

	if len(cfg.DRYSequenceBreakers) > 0 {
		opts = append(opts, service.WithDRYSequenceBreakers(cfg.DRYSequenceBreakers))
	}

	if cfg.XTCProbability != nil && cfg.XTCThreshold != nil {
		opts = append(opts, service.WithXTC(*cfg.XTCProbability, *cfg.XTCThreshold))
	}

	if cfg.Mirostat != nil {
		opts = append(opts, service.WithMirostat(*cfg.Mirostat))
	}

	if cfg.MirostatTau != nil {
		opts = append(opts, service.WithMirostatTau(*cfg.MirostatTau))
	}

	if cfg.MirostatEta != nil {
		opts = append(opts, service.WithMirostatEta(*cfg.MirostatEta))
	}

	if cfg.TypicalP != nil {
		opts = append(opts, service.WithTypicalP(*cfg.TypicalP))
	}

	if cfg.MinKeep != nil {
		opts = append(opts, service.WithMinKeep(*cfg.MinKeep))
	}

	if cfg.DynamicTemperatureRange != nil && cfg.DynamicTemperatureExponent != nil {
		opts = append(opts, service.WithDynamicTemperature(*cfg.DynamicTemperatureRange, *cfg.DynamicTemperatureExponent))
	}

	if cfg.NPrev != nil {
		opts = append(opts, service.WithNPrev(*cfg.NPrev))
	}

	if cfg.NProbs != nil {
		opts = append(opts, service.WithNProbs(*cfg.NProbs))
	}

	if cfg.DebugGeneration {
		opts = append(opts, service.WithDebugGeneration(true))
	}

	if cfg.SpeculativeEnabled {
		opts = append(opts, service.WithSpeculativeEnabled(true))
	}

	if strings.TrimSpace(cfg.SpeculativeDraftModel) != "" {
		opts = append(opts, service.WithSpeculativeDraftModel(cfg.SpeculativeDraftModel))
	}

	if cfg.SpeculativeDraftTokens > 0 {
		opts = append(opts, service.WithSpeculativeDraftTokens(cfg.SpeculativeDraftTokens))
	}

	if cfg.TokenPipelineEnabled {
		opts = append(opts, service.WithTokenPipelineEnabled(true))
	}

	if cfg.ChatAPIEnabled {
		opts = append(opts, service.WithChatAPIEnabled(true))
	}

	if cfg.ChatStreamBufferSize > 0 {
		opts = append(opts, service.WithChatStreamBufferSize(cfg.ChatStreamBufferSize))
	}

	if strings.TrimSpace(cfg.ChatReasoningFormat) != "" {
		opts = append(opts, service.WithChatReasoningFormat(cfg.ChatReasoningFormat))
	}

	if cfg.ChatEnableThinking != nil {
		opts = append(opts, service.WithChatEnableThinking(*cfg.ChatEnableThinking))
	}

	if cfg.ChatReasoningBudget != nil {
		opts = append(opts, service.WithChatReasoningBudget(*cfg.ChatReasoningBudget))
	}

	if cfg.ReinitLlamaLogging {
		opts = append(opts, service.WithReinitLlamaLogging(true))
	}

	if cfg.LogModelStats {
		opts = append(opts, service.WithLogModelStats(true))
	}

	if cfg.ProgressCallback {
		opts = append(opts, service.WithProgressCallback(true))
	}

	svc := service.NewLlamaService(cfg.ModelPath, opts...)

	return NewText(svc), nil
}
