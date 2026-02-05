package domain

import (
	"github.com/magomedcoder/llm-runner/pb"
	"time"
)

type AIChatMessageRole string

const (
	AIChatMessageRoleSystem    AIChatMessageRole = "system"
	AIChatMessageRoleUser      AIChatMessageRole = "user"
	AIChatMessageRoleAssistant AIChatMessageRole = "assistant"
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
	Id               int64
	SessionId        int64
	Content          string
	Role             AIChatMessageRole
	AttachmentName   string
	AttachmentFileId int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
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

func (ai *AIChatMessage) AIToMap() map[string]any {
	return map[string]any{
		"role":    string(ai.Role),
		"content": ai.Content,
	}
}

func AIFromProtoRole(role string) AIChatMessageRole {
	switch role {
	case "system":
		return AIChatMessageRoleSystem
	case "user":
		return AIChatMessageRoleUser
	case "assistant":
		return AIChatMessageRoleAssistant
	default:
		return AIChatMessageRoleUser
	}
}

func AIToProtoRole(role AIChatMessageRole) string {
	return string(role)
}

func AIMessageToProto(msg *AIChatMessage) *pb.ChatMessage {
	if msg == nil {
		return nil
	}
	p := &pb.ChatMessage{
		Id:        msg.Id,
		Content:   msg.Content,
		Role:      AIToProtoRole(msg.Role),
		CreatedAt: msg.CreatedAt.Unix(),
	}
	if msg.AttachmentName != "" {
		p.AttachmentName = &msg.AttachmentName
	}

	return p
}

func AIMessageFromProto(proto *pb.ChatMessage, sessionID int64) *AIChatMessage {
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

	return msg
}
func AIMessagesFromProto(protos []*pb.ChatMessage, sessionID int64) []*AIChatMessage {
	if len(protos) == 0 {
		return nil
	}

	out := make([]*AIChatMessage, len(protos))
	for i, p := range protos {
		out[i] = AIMessageFromProto(p, sessionID)
	}

	return out
}
