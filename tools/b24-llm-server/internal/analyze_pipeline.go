package internal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/tools/b24-llm-server/api/pb/b24llmpb"
)

var (
	errAnalyzeLLM             = errors.New("llm_error")
	errAnalyzeStructuredParse = errors.New("structured_parse_failed")
)

type structuredFail struct {
	msg string
}

func (e *structuredFail) Error() string {
	return e.msg
}

func (e *structuredFail) Unwrap() error {
	return errAnalyzeStructuredParse
}

func analyzeErrorLabel(err error) string {
	var sf *structuredFail
	switch {
	case errors.As(err, &sf):
		return "structured_parse_failed: " + sf.msg
	case errors.Is(err, errAnalyzeLLM):
		return "llm_error"
	default:
		return err.Error()
	}
}

func wrapAnalyzeLLMErr(err error) error {
	return fmt.Errorf("%w: %v", errAnalyzeLLM, err)
}

func buildLLMMessages(req AnalyzeRequest) ([]*domain.Message, error) {
	if strings.TrimSpace(req.Prompt) == "" && strings.TrimSpace(req.AnalysisMode) == "" {
		return nil, fmt.Errorf("prompt_or_mode_required")
	}

	userPrompt := effectiveUserPrompt(req)
	if userPrompt == "" {
		return nil, fmt.Errorf("prompt_required")
	}

	sys := strings.TrimSpace(systemPromptForAnalysisMode(req.AnalysisMode))
	if useStructuredTaskAnalysis(req) {
		sys = sys + "\n\n" + structuredTaskAnalysisSystemAddon()
	}

	messages := []*domain.Message{
		{
			Role:    domain.MessageRoleSystem,
			Content: sys,
		},
		{
			Role:    domain.MessageRoleUser,
			Content: buildTaskContext(req),
		},
	}

	for _, m := range req.History {
		role := strings.TrimSpace(m.Role)
		if role != "user" && role != "assistant" {
			continue
		}

		if strings.TrimSpace(m.Content) == "" {
			continue
		}

		messages = append(messages, &domain.Message{
			Role:    domain.FromProtoRole(role),
			Content: m.Content,
		})
	}

	messages = append(messages, &domain.Message{
		Role:    domain.MessageRoleUser,
		Content: userPrompt,
	})

	return messages, nil
}

func (a *App) analyzeAndCache(ctx context.Context, req AnalyzeRequest) (string, error) {
	cacheKey, ckErr := analyzeCacheKey(req)
	if ckErr == nil {
		if hit, ok := a.respCache.get(cacheKey); ok {
			return hit, nil
		}
	}

	out, err := a.runAnalyzeCore(ctx, req)
	if err != nil {
		return "", err
	}

	if ckErr == nil && strings.TrimSpace(out) != "" {
		a.respCache.set(cacheKey, out)
	}

	return out, nil
}

func (a *App) runAnalyzeCore(ctx context.Context, req AnalyzeRequest) (string, error) {
	messages, err := buildLLMMessages(req)
	if err != nil {
		return "", err
	}

	genParams := generationParamsFromWire(req.Generation)
	stops := stopSequencesFromWire(req.Generation)

	ch, err := a.llm.SendMessage(ctx, 0, a.cfg.Model, messages, stops, 0, genParams)
	if err != nil {
		return "", wrapAnalyzeLLMErr(err)
	}

	out := collectLLMText(ch)
	if useStructuredTaskAnalysis(req) {
		final, ferr := finalizeStructuredTaskAnalysis(ctx, a.llm, a.cfg.Model, genParams, stops, messages, out)
		if ferr != nil {
			return "", &structuredFail{msg: ferr.Error()}
		}

		out = final
	}

	return out, nil
}

func (a *App) emitAnalyzeStream(ctx context.Context, req AnalyzeRequest, send func(*b24llmpb.AnalyzeStreamChunk) error) error {
	messages, err := buildLLMMessages(req)
	if err != nil {
		return send(&b24llmpb.AnalyzeStreamChunk{Error: err.Error()})
	}

	genParams := generationParamsFromWire(req.Generation)
	stops := stopSequencesFromWire(req.Generation)
	model := a.cfg.Model
	cacheKey, ckErr := analyzeCacheKey(req)

	if ckErr == nil {
		if hit, ok := a.respCache.get(cacheKey); ok {
			if err := send(&b24llmpb.AnalyzeStreamChunk{Chunk: hit}); err != nil {
				return err
			}
			return send(&b24llmpb.AnalyzeStreamChunk{Done: true})
		}
	}

	ch, err := a.llm.SendMessage(ctx, 0, model, messages, stops, 0, genParams)
	if err != nil {
		return send(&b24llmpb.AnalyzeStreamChunk{Error: "llm_error"})
	}

	if useStructuredTaskAnalysis(req) {
		var buf strings.Builder
		for chunk := range ch {
			buf.WriteString(chunk.Content)
			buf.WriteString(chunk.ReasoningContent)
			if err := ctx.Err(); err != nil {
				log.Printf("analyze stream: context %v", err)
				return err
			}
		}

		final, ferr := finalizeStructuredTaskAnalysis(ctx, a.llm, model, genParams, stops, messages, strings.TrimSpace(buf.String()))
		if ferr != nil {
			log.Printf("analyze stream structured: %v", ferr)
			return send(&b24llmpb.AnalyzeStreamChunk{Error: "structured_parse_failed"})
		}

		if ckErr == nil && strings.TrimSpace(final) != "" {
			a.respCache.set(cacheKey, final)
		}

		if err := send(&b24llmpb.AnalyzeStreamChunk{Chunk: final}); err != nil {
			return err
		}
		return send(&b24llmpb.AnalyzeStreamChunk{Done: true})
	}

	var acc strings.Builder
	for chunk := range ch {
		acc.WriteString(chunk.Content)
		acc.WriteString(chunk.ReasoningContent)
		if chunk.Content != "" {
			if err := send(&b24llmpb.AnalyzeStreamChunk{Chunk: chunk.Content}); err != nil {
				return err
			}
		}

		if err := ctx.Err(); err != nil {
			log.Printf("analyze stream: context %v", err)
			return err
		}
	}

	full := strings.TrimSpace(acc.String())
	if ckErr == nil && full != "" {
		a.respCache.set(cacheKey, full)
	}

	return send(&b24llmpb.AnalyzeStreamChunk{Done: true})
}
