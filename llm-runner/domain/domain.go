package domain

import (
	"strings"
	"time"

	"github.com/magomedcoder/gen/api/pb/llm-runner/llmrunnerpb"
)

type AIChatMessageRole string

const (
	AIChatMessageRoleSystem    AIChatMessageRole = "system"
	AIChatMessageRoleUser      AIChatMessageRole = "user"
	AIChatMessageRoleAssistant AIChatMessageRole = "assistant"
	AIChatMessageRoleTool      AIChatMessageRole = "tool"
)

type AIChatSession struct {
	Id        int64
	UserId    int
	Title     string
	Model     string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type AIChatMessage struct {
	Id                int64
	SessionId         int64
	Content           string
	Role              AIChatMessageRole
	AttachmentName    string
	AttachmentMime    string
	AttachmentFileId  int64
	AttachmentContent []byte
	ToolCallID        string
	ToolName          string
	ToolCallsJSON     string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
}

func NewAIChatSession(userId int, title string, model string) *AIChatSession {
	return &AIChatSession{
		UserId:    userId,
		Title:     title,
		Model:     model,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func NewAIChatMessage(sessionId int64, content string, role AIChatMessageRole) *AIChatMessage {
	return NewAIChatMessageWithAttachment(sessionId, content, role, "", 0)
}

func NewAIChatMessageWithAttachment(sessionId int64, content string, role AIChatMessageRole, attachmentName string, attachmentFileId int64) *AIChatMessage {
	return &AIChatMessage{
		SessionId:        sessionId,
		Content:          content,
		Role:             role,
		AttachmentName:   attachmentName,
		AttachmentFileId: attachmentFileId,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}

func AIFromProtoRole(role string) AIChatMessageRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "system":
		return AIChatMessageRoleSystem
	case "user":
		return AIChatMessageRoleUser
	case "assistant":
		return AIChatMessageRoleAssistant
	case "tool":
		return AIChatMessageRoleTool
	default:
		return AIChatMessageRoleUser
	}
}

func AIToProtoRole(role AIChatMessageRole) string {
	return string(role)
}

func AIMessageToProto(msg *AIChatMessage) *llmrunnerpb.ChatMessage {
	if msg == nil {
		return nil
	}

	p := &llmrunnerpb.ChatMessage{
		Id:        msg.Id,
		Content:   msg.Content,
		Role:      AIToProtoRole(msg.Role),
		CreatedAt: msg.CreatedAt.Unix(),
	}
	if msg.AttachmentName != "" {
		p.AttachmentName = &msg.AttachmentName
	}
	if len(msg.AttachmentContent) > 0 {
		p.AttachmentContent = msg.AttachmentContent
	}

	if mime := strings.TrimSpace(msg.AttachmentMime); mime != "" {
		p.AttachmentMime = &mime
	}

	if msg.ToolCallID != "" {
		p.ToolCallId = &msg.ToolCallID
	}

	if msg.ToolName != "" {
		p.ToolName = &msg.ToolName
	}

	if msg.ToolCallsJSON != "" {
		p.ToolCallsJson = &msg.ToolCallsJSON
	}

	return p
}

func AIMessageFromProto(proto *llmrunnerpb.ChatMessage, sessionID int64) *AIChatMessage {
	if proto == nil {
		return nil
	}

	msg := &AIChatMessage{
		Id:        proto.Id,
		SessionId: sessionID,
		Content:   proto.Content,
		Role:      AIFromProtoRole(proto.Role),
		CreatedAt: time.Unix(proto.CreatedAt, 0),
		UpdatedAt: time.Unix(proto.CreatedAt, 0),
	}
	if proto.AttachmentName != nil {
		msg.AttachmentName = *proto.AttachmentName
	}
	if len(proto.AttachmentContent) > 0 {
		msg.AttachmentContent = append([]byte(nil), proto.AttachmentContent...)
	}

	if proto.AttachmentMime != nil {
		msg.AttachmentMime = strings.TrimSpace(*proto.AttachmentMime)
	}

	if proto.ToolCallId != nil {
		msg.ToolCallID = strings.TrimSpace(*proto.ToolCallId)
	}

	if proto.ToolName != nil {
		msg.ToolName = strings.TrimSpace(*proto.ToolName)
	}

	if proto.ToolCallsJson != nil {
		msg.ToolCallsJSON = strings.TrimSpace(*proto.ToolCallsJson)
	}

	return msg
}
func AIMessagesFromProto(protos []*llmrunnerpb.ChatMessage, sessionID int64) []*AIChatMessage {
	if len(protos) == 0 {
		return nil
	}

	out := make([]*AIChatMessage, len(protos))
	for i, p := range protos {
		out[i] = AIMessageFromProto(p, sessionID)
	}

	return out
}
