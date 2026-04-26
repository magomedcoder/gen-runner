package mappers

import (
	"strings"

	"github.com/magomedcoder/gen/api/pb/app/chatpb"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/document"
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

	if mime := strings.TrimSpace(msg.AttachmentMime); mime != "" {
		p.AttachmentMime = &mime
	} else if msg.AttachmentFileID != nil && document.IsImageAttachment(msg.AttachmentName) {
		if inferred := document.ImageMIMEFromFilename(msg.AttachmentName); inferred != "" {
			p.AttachmentMime = &inferred
		}
	}

	return p
}
