package usecase

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
)

func normalizedAvailableModels(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, m := range raw {
		v := strings.TrimSpace(m)
		if v == "" {
			continue
		}

		if _, ok := seen[v]; ok {
			continue
		}

		seen[v] = struct{}{}
		out = append(out, v)
	}

	return out
}

func resolveModelForUser(
	ctx context.Context,
	llmRepo domain.LLMRepository,
	preferenceRepo domain.ChatPreferenceRepository,
	userID int,
	requestedModel string,
	sessionModel string,
) (string, error) {
	availableRaw, err := llmRepo.GetModels(ctx)
	if err != nil {
		return "", err
	}
	available := normalizedAvailableModels(availableRaw)
	if len(available) == 0 {
		return "", fmt.Errorf("нет доступных моделей на активных раннерах")
	}

	requested := strings.TrimSpace(requestedModel)
	if requested != "" {
		if slices.Contains(available, requested) {
			return requested, nil
		}

		return "", fmt.Errorf("модель %q недоступна (доступные: %s)", requested, strings.Join(available, ", "))
	}

	fromSession := strings.TrimSpace(sessionModel)
	if fromSession != "" && slices.Contains(available, fromSession) {
		return fromSession, nil
	}

	selectedRunner, err := preferenceRepo.GetSelectedRunner(ctx, userID)
	if err == nil && strings.TrimSpace(selectedRunner) != "" {
		defaultModel, err := preferenceRepo.GetDefaultRunnerModel(ctx, userID, selectedRunner)
		if err == nil {
			defaultModel = strings.TrimSpace(defaultModel)
			if defaultModel != "" && slices.Contains(available, defaultModel) {
				return defaultModel, nil
			}
		}
	}

	return available[0], nil
}
