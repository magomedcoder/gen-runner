package usecase

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
)

func (c *ChatUseCase) aggregateMaxContextTokens() int {
	maxTok := 0
	if c.runnerReg != nil {
		maxTok = c.runnerReg.AggregateChatHints().MaxContextTokens
	}

	if maxTok <= 0 && c.llmContextFallbackTokens > 0 {
		maxTok = normalizeLLMHistoryApproxMaxTokens(c.llmContextFallbackTokens)
	}

	return maxTok
}

func (c *ChatUseCase) effectiveMaxRAGContextRunes(systemAndHistory []*domain.Message, userMessage string) int {
	cap := maxFileRAGContextRunesCeiling
	maxTok := c.aggregateMaxContextTokens()
	if maxTok <= 0 {
		const blindRAGRunes = 3200
		if blindRAGRunes < cap {
			return blindRAGRunes
		}

		return cap
	}

	pre := sumApproxTokens(systemAndHistory)
	const genReserve = 512
	userOverhead := max(utf8.RuneCountInString(strings.TrimSpace(userMessage))/2, 32)

	ragTok := max(maxTok-pre-genReserve-userOverhead, 120)

	runesLimit := min(ragTok*2, cap)

	if runesLimit < 200 {
		runesLimit = 200
	}

	return runesLimit
}

func (c *ChatUseCase) capLLMHistoryTokens(ctx context.Context, msgs []*domain.Message, tailPreserve int, sessionID int64, resolvedModel string, chatRunnerAddr string, allowSummarize bool) ([]*domain.Message, bool) {
	maxTok := c.aggregateMaxContextTokens()
	if maxTok <= 0 {
		return msgs, false
	}

	out, trimmed, dropped := trimLLMMessagesByApproxTokensWithDropped(msgs, maxTok, tailPreserve)
	if !trimmed {
		return out, false
	}

	summarizeDropped := false
	if c.runnerReg != nil {
		summarizeDropped = c.runnerReg.AggregateChatHints().LLMHistorySummarizeDropped
	}

	logger.I("ChatUseCase: сессия=%d промпт усечён по оценке токенов (~лимит %d): сообщений %d -> %d", sessionID, maxTok, len(msgs), len(out))
	if allowSummarize && summarizeDropped && len(dropped) > 0 {
		if sum := strings.TrimSpace(c.summarizeDroppedMessages(ctx, sessionID, resolvedModel, chatRunnerAddr, dropped)); sum != "" {
			out = injectSummaryAfterSystem(out, sum)
			out2, trimmedAgain, _ := trimLLMMessagesByApproxTokensWithDropped(out, maxTok, tailPreserve)
			if trimmedAgain {
				logger.W("ChatUseCase: сессия=%d после вставки резюме снова усечено: %d -> %d сообщений", sessionID, len(out), len(out2))
			}

			out = out2
		}
	}

	return out, true
}

func filterHistoryForLLM(messages []*domain.Message) []*domain.Message {
	if len(messages) == 0 {
		return nil
	}

	out := make([]*domain.Message, 0, len(messages))
	for _, m := range messages {
		if m == nil {
			continue
		}

		if m.Role == domain.MessageRoleAssistant && strings.TrimSpace(m.Content) == "" {
			if strings.TrimSpace(m.ToolCallsJSON) == "" {
				continue
			}
		}

		out = append(out, m)
	}

	return out
}
