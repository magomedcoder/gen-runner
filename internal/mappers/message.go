package mappers

import (
	"strings"
	"time"

	"github.com/magomedcoder/gen/api/pb/chatpb"
	"github.com/magomedcoder/gen/internal/domain"
)

func MessageToProto(msg *domain.Message) *chatpb.ChatMessage {
	if msg == nil {
		return nil
	}

	p := &chatpb.ChatMessage{
		Id:        msg.Id,
		Content:   msg.Content,
		Role:      domain.ToProtoRole(msg.Role),
		CreatedAt: msg.CreatedAt.Unix(),
	}
	if msg.AttachmentName != "" {
		p.AttachmentName = &msg.AttachmentName
	}
	if msg.ToolCallID != "" {
		v := msg.ToolCallID
		p.ToolCallId = &v
	}
	if msg.ToolName != "" {
		v := msg.ToolName
		p.ToolName = &v
	}
	if msg.ToolCallsJSON != "" {
		v := msg.ToolCallsJSON
		p.ToolCallsJson = &v
	}
	return p
}

func MessagesFromProto(pbMsgs []*chatpb.ChatMessage, sessionID int64) []*domain.Message {
	if len(pbMsgs) == 0 {
		return nil
	}
	out := make([]*domain.Message, 0, len(pbMsgs))
	for _, m := range pbMsgs {
		if m == nil {
			continue
		}
		var createdAt time.Time
		if m.CreatedAt != 0 {
			createdAt = time.Unix(m.CreatedAt, 0)
		}
		msg := &domain.Message{
			Id:        m.Id,
			SessionId: sessionID,
			Content:   m.Content,
			Role:      domain.FromProtoRole(m.Role),
			CreatedAt: createdAt,
		}
		if m.AttachmentName != nil {
			msg.AttachmentName = *m.AttachmentName
		}
		if m.ToolCallId != nil {
			msg.ToolCallID = strings.TrimSpace(*m.ToolCallId)
		}
		if m.ToolName != nil {
			msg.ToolName = strings.TrimSpace(*m.ToolName)
		}
		if m.ToolCallsJson != nil {
			msg.ToolCallsJSON = strings.TrimSpace(*m.ToolCallsJson)
		}
		out = append(out, msg)
	}
	return out
}
