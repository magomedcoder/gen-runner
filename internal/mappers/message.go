package mappers

import (
	"github.com/magomedcoder/gen/api/pb"
	"github.com/magomedcoder/gen/internal/domain"
)

func MessageToProto(msg *domain.Message) *pb.ChatMessage {
	if msg == nil {
		return nil
	}

	return &pb.ChatMessage{
		Id:        msg.Id,
		Content:   msg.Content,
		Role:      domain.ToProtoRole(msg.Role),
		CreatedAt: msg.CreatedAt.Unix(),
	}
}
