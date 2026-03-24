package domain

import "strings"

type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
)

func (m *Message) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"role":    string(m.Role),
		"content": m.Content,
	}
}

func FromProtoRole(role string) MessageRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "system":
		return MessageRoleSystem
	case "user":
		return MessageRoleUser
	case "assistant":
		return MessageRoleAssistant
	case "tool":
		return MessageRoleTool
	default:
		return MessageRoleUser
	}
}

func ToProtoRole(role MessageRole) string {
	return string(role)
}
