package usecase

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
)

const HistoryTruncatedClientNotice = "Часть более старой переписки не передана модели из-за лимита оценки токенов."

func approximateLLMMessageTokens(m *domain.Message) int {
	if m == nil {
		return 0
	}

	runes := utf8.RuneCountInString(m.Content)
	runes += utf8.RuneCountInString(m.ToolCallsJSON)
	runes += utf8.RuneCountInString(m.ToolName)
	runes += utf8.RuneCountInString(m.ToolCallID)
	if len(m.AttachmentContent) > 0 {
		runes += len(m.AttachmentContent) / 4
	} else {
		runes += utf8.RuneCountInString(m.AttachmentName)
	}

	const perMessageOverhead = 6
	tok := runes/4 + perMessageOverhead
	if tok < 1 {
		return 1
	}

	return tok
}

func sumApproxTokens(msgs []*domain.Message) int {
	s := 0
	for _, m := range msgs {
		s += approximateLLMMessageTokens(m)
	}

	return s
}

func trimLLMMessagesByApproxTokens(msgs []*domain.Message, maxTokens int, tailPreserve int) ([]*domain.Message, bool) {
	out, trimmed, _ := trimLLMMessagesByApproxTokensWithDropped(msgs, maxTokens, tailPreserve)
	return out, trimmed
}

func trimLLMMessagesByApproxTokensWithDropped(msgs []*domain.Message, maxTokens int, tailPreserve int) ([]*domain.Message, bool, []*domain.Message) {
	if maxTokens <= 0 || len(msgs) <= 1 {
		return msgs, false, nil
	}

	if tailPreserve < 0 {
		tailPreserve = 0
	}

	if len(msgs) <= 1+tailPreserve {
		total := sumApproxTokens(msgs)
		if total <= maxTokens {
			return msgs, false, nil
		}

		out := cloneMessageSliceForTrim(msgs)
		out = shrinkMessagesToApproxTokenBudget(out, maxTokens)
		return out, true, nil
	}

	tailStart := len(msgs) - tailPreserve
	system := msgs[0]
	tail := msgs[tailStart:]
	middle := append([]*domain.Message(nil), msgs[1:tailStart]...)

	total := approximateLLMMessageTokens(system) + sumApproxTokens(middle) + sumApproxTokens(tail)
	if total <= maxTokens {
		return msgs, false, nil
	}

	var dropped []*domain.Message
	trimmed := false
	for len(middle) > 0 && total > maxTokens {
		dropped = append(dropped, middle[0])
		drop := approximateLLMMessageTokens(middle[0])
		middle = middle[1:]
		total -= drop
		trimmed = true
	}

	if !trimmed {
		return msgs, false, nil
	}

	out := make([]*domain.Message, 0, 1+len(middle)+len(tail))
	out = append(out, system)
	out = append(out, middle...)
	out = append(out, tail...)

	return out, true, dropped
}

const llmContextTruncationNotice = "\n\n[…фрагмент убран из‑за лимита контекста модели…]"

func cloneMessageSliceForTrim(msgs []*domain.Message) []*domain.Message {
	out := make([]*domain.Message, len(msgs))
	for i, m := range msgs {
		if m == nil {
			continue
		}

		cp := *m
		out[i] = &cp
	}

	return out
}

func shrinkMessagesToApproxTokenBudget(msgs []*domain.Message, maxTokens int) []*domain.Message {
	if len(msgs) == 0 {
		return msgs
	}

	out := cloneMessageSliceForTrim(msgs)
	for sumApproxTokens(out) > maxTokens {
		idx := -1
		for i := len(out) - 1; i >= 0; i-- {
			if out[i] != nil && strings.TrimSpace(out[i].Content) != "" {
				idx = i
				break
			}
		}

		if idx < 0 {
			break
		}

		r := []rune(out[idx].Content)
		if len(r) <= 96 {
			out[idx].Content = ""
			continue
		}

		newLen := max(len(r)*3/4, 64)

		out[idx].Content = string(r[:newLen]) + llmContextTruncationNotice
	}
	return out
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}

	return string(r[:maxRunes]) + "\n..."
}

func buildDroppedDialoguePlainText(msgs []*domain.Message, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = math.MaxInt
	}

	var b strings.Builder
	used := 0
	for _, m := range msgs {
		if m == nil {
			continue
		}

		role := string(m.Role)
		content := strings.TrimSpace(m.Content)
		if m.Role == domain.MessageRoleAssistant && strings.TrimSpace(m.ToolCallsJSON) != "" {
			content = strings.TrimSpace(m.ToolCallsJSON) + "\n" + content
		}

		if m.Role == domain.MessageRoleTool {
			tn := strings.TrimSpace(m.ToolName)
			if tn != "" {
				content = "[" + tn + "] " + content
			}
		}

		budget := maxRunes - used - utf8.RuneCountInString(role) - 4
		if budget < 80 {
			break
		}

		if utf8.RuneCountInString(content) > budget {
			content = truncateRunes(content, budget)
		}

		line := role + ": " + content + "\n"
		rn := utf8.RuneCountInString(line)
		if used+rn > maxRunes {
			break
		}

		b.WriteString(line)
		used += rn
	}

	return strings.TrimSpace(b.String())
}

func injectSummaryAfterSystem(msgs []*domain.Message, summary string) []*domain.Message {
	if len(msgs) == 0 || strings.TrimSpace(summary) == "" {
		return msgs
	}

	sys := *msgs[0]
	sys.Content = strings.TrimSpace(sys.Content) + "\n\n--- Резюме отброшенной части диалога ---\n" + strings.TrimSpace(summary)
	out := make([]*domain.Message, len(msgs))
	out[0] = &sys
	copy(out[1:], msgs[1:])

	return out
}

func normalizeLLMHistoryApproxMaxTokens(n int) int {
	if n <= 0 {
		return 0
	}

	if n < 512 {
		return 512
	}

	if n > 500_000 {
		return 500_000
	}

	return n
}
