package mcpclient

import (
	"context"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxSamplingTokens     int32 = 8192
	maxSamplingReplyRunes       = 64000
)

var samplingEnabled atomic.Bool

func SetSamplingEnabled(v bool) {
	samplingEnabled.Store(v)
}

func SamplingEnabled() bool {
	return samplingEnabled.Load()
}

type samplingRunnerCtxKey struct{}

type SamplingRunner struct {
	LLM            domain.LLMRepository
	SessionID      int64
	RunnerAddr     string
	Model          string
	StopSequences  []string
	TimeoutSeconds int32
	GenParams      *domain.GenerationParams
}

func WithSamplingRunner(ctx context.Context, sr *SamplingRunner) context.Context {
	if sr == nil {
		return ctx
	}

	return context.WithValue(ctx, samplingRunnerCtxKey{}, sr)
}

func samplingRunnerFromCtx(ctx context.Context) *SamplingRunner {
	v, _ := ctx.Value(samplingRunnerCtxKey{}).(*SamplingRunner)
	return v
}

func samplingMessagesToDomain(sessionID int64, systemPrompt string, msgs []*mcp.SamplingMessage) []*domain.Message {
	var out []*domain.Message
	if sp := strings.TrimSpace(systemPrompt); sp != "" {
		out = append(out, domain.NewMessage(sessionID, sp, domain.MessageRoleSystem))
	}

	for _, m := range msgs {
		if m == nil {
			continue
		}

		content := strings.TrimSpace(contentToLLMString(m.Content))
		role := domain.FromProtoRole(string(m.Role))
		out = append(out, domain.NewMessage(sessionID, content, role))
	}

	return out
}

func samplingMessagesV2ToDomain(sessionID int64, systemPrompt string, msgs []*mcp.SamplingMessageV2) []*domain.Message {
	var out []*domain.Message
	if sp := strings.TrimSpace(systemPrompt); sp != "" {
		out = append(out, domain.NewMessage(sessionID, sp, domain.MessageRoleSystem))
	}

	for _, m := range msgs {
		if m == nil {
			continue
		}

		var parts []string
		for _, c := range m.Content {
			if c == nil {
				continue
			}

			if s := strings.TrimSpace(contentToLLMString(c)); s != "" {
				parts = append(parts, s)
			}
		}

		content := strings.Join(parts, "\n")
		role := domain.FromProtoRole(string(m.Role))
		out = append(out, domain.NewMessage(sessionID, content, role))
	}

	return out
}

func (sr *SamplingRunner) runCompletion(ctx context.Context, msgs []*domain.Message, maxTokens int64, temperature float64, extraStops []string) (string, error) {
	if sr == nil || sr.LLM == nil {
		return "", context.Canceled
	}

	var gp domain.GenerationParams
	if sr.GenParams != nil {
		gp = *sr.GenParams
	}

	gp.Tools = nil
	gp.ResponseFormat = nil

	if maxTokens > 0 {
		mt := min(int32(maxTokens), maxSamplingTokens)

		gp.MaxTokens = &mt
	}

	if temperature > 0 {
		t := float32(temperature)
		gp.Temperature = &t
	}

	stops := sr.StopSequences
	if len(extraStops) > 0 {
		stops = append(append([]string{}, stops...), extraStops...)
	}

	ch, err := sr.LLM.SendMessageOnRunner(ctx, sr.RunnerAddr, sr.SessionID, sr.Model, msgs, stops, sr.TimeoutSeconds, &gp)
	if err != nil {
		return "", err
	}

	text, err := drainSamplingStream(ctx, ch)
	if err != nil {
		return text, err
	}

	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > maxSamplingReplyRunes {
		r := []rune(text)
		text = string(r[:maxSamplingReplyRunes]) + "\n...(обрезано для MCP sampling)"
	}

	return text, nil
}

func drainSamplingStream(ctx context.Context, ch chan domain.LLMStreamChunk) (string, error) {
	var b strings.Builder
	for {
		select {
		case <-ctx.Done():
			return b.String(), ctx.Err()
		case c, ok := <-ch:
			if !ok {
				return b.String(), nil
			}

			b.WriteString(c.Content)
		}
	}
}

func runSamplingCreateMessage(ctx context.Context, sr *SamplingRunner, p *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
	if p == nil {
		p = &mcp.CreateMessageParams{}
	}

	msgs := samplingMessagesToDomain(sr.SessionID, p.SystemPrompt, p.Messages)
	logger.I("MCP sampling/createMessage: session=%d messages=%d maxTokens=%d", sr.SessionID, len(msgs), p.MaxTokens)

	text, err := sr.runCompletion(ctx, msgs, p.MaxTokens, p.Temperature, p.StopSequences)
	if err != nil {
		logger.W("MCP sampling/createMessage: %v", err)
		return nil, err
	}

	return &mcp.CreateMessageResult{
		Content:    &mcp.TextContent{Text: text},
		Model:      sr.Model,
		Role:       "assistant",
		StopReason: "endTurn",
	}, nil
}

func runSamplingCreateMessageWithTools(ctx context.Context, sr *SamplingRunner, p *mcp.CreateMessageWithToolsParams) (*mcp.CreateMessageWithToolsResult, error) {
	if p == nil {
		p = &mcp.CreateMessageWithToolsParams{}
	}

	msgs := samplingMessagesV2ToDomain(sr.SessionID, p.SystemPrompt, p.Messages)
	logger.I("MCP sampling/createMessage (tools form): session=%d messages=%d serverTools=%d - инструменты sampling в LLM не проксируются",
		sr.SessionID, len(msgs), len(p.Tools))

	text, err := sr.runCompletion(ctx, msgs, p.MaxTokens, p.Temperature, p.StopSequences)
	if err != nil {
		logger.W("MCP sampling/createMessage tools: %v", err)
		return nil, err
	}

	return &mcp.CreateMessageWithToolsResult{
		Content:    []mcp.Content{&mcp.TextContent{Text: text}},
		Model:      sr.Model,
		Role:       "assistant",
		StopReason: "endTurn",
	}, nil
}

func samplingClientOptions(ctx context.Context) *mcp.ClientOptions {
	if !samplingEnabled.Load() {
		return nil
	}

	sr := samplingRunnerFromCtx(ctx)
	if sr == nil {
		return nil
	}

	runner := *sr
	return &mcp.ClientOptions{
		CreateMessageHandler: func(cctx context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			if req == nil {
				return nil, context.Canceled
			}

			return runSamplingCreateMessage(cctx, &runner, req.Params)
		},
		CreateMessageWithToolsHandler: func(cctx context.Context, req *mcp.CreateMessageWithToolsRequest) (*mcp.CreateMessageWithToolsResult, error) {
			if req == nil {
				return nil, context.Canceled
			}

			return runSamplingCreateMessageWithTools(cctx, &runner, req.Params)
		},
	}
}
