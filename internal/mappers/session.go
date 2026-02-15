package mappers

import (
	"github.com/magomedcoder/gen/api/pb"
	"github.com/magomedcoder/gen/internal/domain"
)

func SessionToProto(session *domain.ChatSession) *pb.ChatSession {
	if session == nil {
		return nil
	}

	return &pb.ChatSession{
		Id:        session.Id,
		Title:     session.Title,
		CreatedAt: session.CreatedAt.Unix(),
		UpdatedAt: session.UpdatedAt.Unix(),
	}
}
