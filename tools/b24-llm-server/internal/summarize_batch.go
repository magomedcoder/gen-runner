package internal

import (
	"context"
	"fmt"
	"strings"
)

type SummarizeBatchRequest struct {
	Items      []AnalyzeRequest `json:"items"`
	Generation *GenerationWire  `json:"generation,omitempty"`
}

type SummarizeBatchResult struct {
	TaskID  string `json:"task_id"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type SummarizeBatchResponse struct {
	Results []SummarizeBatchResult `json:"results"`
}

func mergeBatchItemGeneration(batch SummarizeBatchRequest, item AnalyzeRequest) AnalyzeRequest {
	if batch.Generation != nil && item.Generation == nil {
		g := *batch.Generation
		item.Generation = &g
	}

	return item
}

func validateSummarizeBatchRequest(req *SummarizeBatchRequest, maxItems int) error {
	if len(req.Items) == 0 {
		return fmt.Errorf("items_required")
	}

	if len(req.Items) > maxItems {
		return fmt.Errorf("too_many_items")
	}

	for i := range req.Items {
		merged := mergeBatchItemGeneration(*req, req.Items[i])
		if _, err := buildLLMMessages(merged); err != nil {
			return fmt.Errorf("item_%d: %w", i, err)
		}
	}

	return nil
}

func (a *App) RunSummarizeBatch(ctx context.Context, req *SummarizeBatchRequest) (SummarizeBatchResponse, error) {
	if err := validateSummarizeBatchRequest(req, a.cfg.MaxSummarizeBatchItems); err != nil {
		return SummarizeBatchResponse{}, err
	}

	results := make([]SummarizeBatchResult, 0, len(req.Items))
	for _, item := range req.Items {
		merged := mergeBatchItemGeneration(*req, item)
		tid := strings.TrimSpace(merged.TaskID)

		cacheKey, ckErr := analyzeCacheKey(merged)
		if ckErr == nil {
			if hit, ok := a.respCache.get(cacheKey); ok {
				results = append(results, SummarizeBatchResult{
					TaskID: tid,
					Message: hit,
				})
				continue
			}
		}

		out, err := a.runAnalyzeCore(ctx, merged)
		if err != nil {
			results = append(results, SummarizeBatchResult{
				TaskID: tid,
				Error:  analyzeErrorLabel(err),
			})
			continue
		}

		if ckErr == nil && strings.TrimSpace(out) != "" {
			a.respCache.set(cacheKey, out)
		}

		results = append(results, SummarizeBatchResult{
			TaskID:  tid,
			Message: out,
		})
	}

	return SummarizeBatchResponse{Results: results}, nil
}
