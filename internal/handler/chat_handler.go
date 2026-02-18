package handler

import (
	"context"
	"time"

	"github.com/magomedcoder/gen/api/pb"
	"github.com/magomedcoder/gen/internal/mappers"
	"github.com/magomedcoder/gen/internal/usecase"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ChatHandler struct {
	pb.UnimplementedChatServiceServer
	chatUseCase *usecase.ChatUseCase
	authUseCase *usecase.AuthUseCase
}

func NewChatHandler(chatUseCase *usecase.ChatUseCase, authUseCase *usecase.AuthUseCase) *ChatHandler {
	return &ChatHandler{
		chatUseCase: chatUseCase,
		authUseCase: authUseCase,
	}
}

func (c *ChatHandler) getUserID(ctx context.Context) (int, error) {
	user, err := getUserFromContext(ctx, c.authUseCase)
	if err != nil {
		return 0, err
	}
	return user.Id, nil
}

func (c *ChatHandler) SendMessage(req *pb.SendMessageRequest, stream pb.ChatService_SendMessageServer) error {
	ctx := stream.Context()
	userID, err := c.getUserID(ctx)
	if err != nil {
		return err
	}

	if len(req.Messages) == 0 {
		return status.Error(codes.InvalidArgument, "сообщения не предоставлены")
	}

	lastMessage := req.Messages[len(req.Messages)-1]
	userMessage := lastMessage.Content

	responseChan, messageId, err := c.chatUseCase.SendMessage(ctx, userID, req.SessionId, req.GetModel(), userMessage)
	if err != nil {
		return ToStatusError(codes.Internal, err)
	}

	createdAt := time.Now().Unix()

	for chunk := range responseChan {
		err := stream.Send(&pb.ChatResponse{
			Id:        messageId,
			Content:   chunk,
			Role:      "assistant",
			CreatedAt: createdAt,
			Done:      false,
		})
		if err != nil {
			return err
		}
	}

	return stream.Send(&pb.ChatResponse{
		Id:        messageId,
		Content:   "",
		Role:      "assistant",
		CreatedAt: createdAt,
		Done:      true,
	})
}

func (c *ChatHandler) CreateSession(ctx context.Context, req *pb.CreateSessionRequest) (*pb.ChatSession, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	session, err := c.chatUseCase.CreateSession(ctx, userID, req.GetTitle(), req.GetModel())
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	return mappers.SessionToProto(session), nil
}

func (c *ChatHandler) GetSession(ctx context.Context, req *pb.GetSessionRequest) (*pb.ChatSession, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	session, err := c.chatUseCase.GetSession(ctx, userID, req.SessionId)
	if err != nil {
		return nil, ToStatusError(codes.NotFound, err)
	}

	return mappers.SessionToProto(session), nil
}

func (c *ChatHandler) GetSessions(ctx context.Context, req *pb.GetSessionsRequest) (*pb.GetSessionsResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	page, pageSize := normalizePagination(req.Page, req.PageSize, 20)

	sessions, total, err := c.chatUseCase.GetSessions(ctx, userID, page, pageSize)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	protoSessions := make([]*pb.ChatSession, len(sessions))
	for i, session := range sessions {
		protoSessions[i] = mappers.SessionToProto(session)
	}

	return &pb.GetSessionsResponse{
		Sessions: protoSessions,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (c *ChatHandler) GetSessionMessages(ctx context.Context, req *pb.GetSessionMessagesRequest) (*pb.GetSessionMessagesResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	page, pageSize := normalizePagination(req.Page, req.PageSize, 50)

	messages, total, err := c.chatUseCase.GetSessionMessages(ctx, userID, req.SessionId, page, pageSize)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	protoMessages := make([]*pb.ChatMessage, len(messages))
	for i, msg := range messages {
		protoMessages[i] = mappers.MessageToProto(msg)
	}

	return &pb.GetSessionMessagesResponse{
		Messages: protoMessages,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (c *ChatHandler) DeleteSession(ctx context.Context, req *pb.DeleteSessionRequest) (*pb.Empty, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := c.chatUseCase.DeleteSession(ctx, userID, req.SessionId); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	return &pb.Empty{}, nil
}

func (c *ChatHandler) UpdateSessionTitle(ctx context.Context, req *pb.UpdateSessionTitleRequest) (*pb.ChatSession, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	session, err := c.chatUseCase.UpdateSessionTitle(ctx, userID, req.SessionId, req.Title)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	return mappers.SessionToProto(session), nil
}

func (c *ChatHandler) UpdateSessionModel(ctx context.Context, req *pb.UpdateSessionModelRequest) (*pb.ChatSession, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	session, err := c.chatUseCase.UpdateSessionModel(ctx, userID, req.SessionId, req.GetModel())
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	return mappers.SessionToProto(session), nil
}

func (c *ChatHandler) CheckConnection(ctx context.Context, req *pb.Empty) (*pb.ConnectionResponse, error) {
	return &pb.ConnectionResponse{IsConnected: true}, nil
}

func (c *ChatHandler) GetModels(ctx context.Context, req *pb.Empty) (*pb.GetModelsResponse, error) {
	models, err := c.chatUseCase.GetModels(ctx)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	return &pb.GetModelsResponse{Models: models}, nil
}
