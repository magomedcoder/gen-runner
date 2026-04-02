package mappers

import (
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
		UpdatedAt: msg.UpdatedAt.Unix(),
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

	if msg.AttachmentFileID != nil {
		v := *msg.AttachmentFileID
		p.AttachmentFileId = &v
	}

	return p
}
