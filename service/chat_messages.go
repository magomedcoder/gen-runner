package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/magomedcoder/gen-runner/domain"
)

func messageHasPayload(m *domain.AIChatMessage) bool {
	if m == nil {
		return false
	}

	if len(m.AttachmentContent) > 0 {
		return true
	}

	if strings.TrimSpace(m.Content) != "" {
		return true
	}

	if m.Role == domain.AIChatMessageRoleAssistant && strings.TrimSpace(m.ToolCallsJSON) != "" {
		return true
	}

	if m.Role == domain.AIChatMessageRoleTool && strings.TrimSpace(m.ToolCallID) != "" {
		return true
	}

	return false
}

func FormatContentForBuiltinChatTemplate(m *domain.AIChatMessage) string {
	if m == nil {
		return ""
	}

	c := m.Content
	if m.Role == domain.AIChatMessageRoleTool {
		var b strings.Builder
		if m.ToolCallID != "" {
			fmt.Fprintf(&b, "[call_id=%s] ", m.ToolCallID)
		}

		if m.ToolName != "" {
			fmt.Fprintf(&b, "[%s] ", m.ToolName)
		}

		b.WriteString(c)
		return b.String()
	}

	if m.Role == domain.AIChatMessageRoleAssistant && strings.TrimSpace(m.ToolCallsJSON) != "" {
		if strings.TrimSpace(c) != "" {
			return c + "\n[tool_calls]: " + m.ToolCallsJSON
		}

		return "[tool_calls]: " + m.ToolCallsJSON
	}

	return c
}

func isImageFilenameExt(name string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(name))) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func messageLikelyVisionImageAttachment(m *domain.AIChatMessage) bool {
	if m == nil || len(m.AttachmentContent) == 0 {
		return false
	}

	mt := strings.ToLower(strings.TrimSpace(m.AttachmentMime))
	if strings.HasPrefix(mt, "image/") {
		return true
	}

	if mt == "" {
		return isImageFilenameExt(m.AttachmentName) || strings.TrimSpace(m.AttachmentName) == ""
	}

	return false
}

func MessagesHaveVisionAttachments(messages []*domain.AIChatMessage) bool {
	for _, m := range messages {
		if messageLikelyVisionImageAttachment(m) {
			return true
		}
	}

	return false
}

func NormalizeChatMessages(messages []*domain.AIChatMessage) []*domain.AIChatMessage {
	if len(messages) == 0 {
		return nil
	}

	var systemParts []string
	var rest []*domain.AIChatMessage

	for _, m := range messages {
		if m == nil {
			continue
		}

		if !messageHasPayload(m) {
			continue
		}

		switch m.Role {
		case domain.AIChatMessageRoleSystem:
			systemParts = append(systemParts, strings.TrimSpace(m.Content))
		default:
			rest = append(rest, m)
		}
	}

	var out []*domain.AIChatMessage
	if len(systemParts) > 0 {
		merged := strings.Join(systemParts, "\n\n")
		out = append(out, domain.NewAIChatMessage(0, merged, domain.AIChatMessageRoleSystem))
	}
	out = append(out, rest...)

	return out
}

func MergeStopSequences(client []string, preset []string) []string {
	seen := make(map[string]struct{}, len(client)+len(preset))
	out := make([]string, 0, len(client)+len(preset))
	for _, s := range client {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}

		if _, ok := seen[s]; ok {
			continue
		}

		seen[s] = struct{}{}
		out = append(out, s)
	}

	for _, s := range preset {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}

		if _, ok := seen[s]; ok {
			continue
		}

		seen[s] = struct{}{}
		out = append(out, s)
	}

	return out
}

func fallbackPlainChatPrompt(messages []*domain.AIChatMessage, genParams *domain.GenerationParams) string {
	var b strings.Builder
	for _, m := range messages {
		if m == nil {
			continue
		}

		var role string
		switch m.Role {
		case domain.AIChatMessageRoleSystem:
			role = "System"
		case domain.AIChatMessageRoleAssistant:
			role = "Assistant"
		case domain.AIChatMessageRoleTool:
			role = "Tool"
		default:
			role = "User"
		}

		b.WriteString(role)
		b.WriteString(": ")
		if m.Role == domain.AIChatMessageRoleTool {
			if m.ToolCallID != "" {
				b.WriteString("(call_id=")
				b.WriteString(m.ToolCallID)
				b.WriteString(") ")
			}
			if m.ToolName != "" {
				b.WriteString("[")
				b.WriteString(m.ToolName)
				b.WriteString("] ")
			}
		}
		b.WriteString(FormatContentForBuiltinChatTemplate(m))
		b.WriteString("\n")
	}

	if genParams != nil && len(genParams.Tools) > 0 {
		b.WriteString(fallbackToolsBlock(genParams.Tools))
	}
	b.WriteString("Assistant: ")

	return b.String()
}

func fallbackToolsBlock(tools []domain.Tool) string {
	var b strings.Builder
	b.WriteString("\nTools:\n")
	for _, t := range tools {
		b.WriteString("- ")
		b.WriteString(t.Name)
		if t.Description != "" {
			b.WriteString(": ")
			b.WriteString(t.Description)
		}

		if t.ParametersJSON != "" {
			b.WriteString(" (params: ")
			b.WriteString(t.ParametersJSON)
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	b.WriteString("\nЧтобы вызвать инструмент, верни один JSON-массив (можно в блоке ```json), строго в формате:\n")
	b.WriteString(`[{"tool_name":"<имя из списка>","parameters":{...}}]`)
	b.WriteString("\n\nПоле parameters - объект JSON; если параметров нет, используй {}.\n\n")

	return b.String()
}
