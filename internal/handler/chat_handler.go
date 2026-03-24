package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/magomedcoder/gen/api/pb/chatpb"
	"github.com/magomedcoder/gen/api/pb/commonpb"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mappers"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ChatHandler struct {
	chatpb.UnimplementedChatServiceServer
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
	user, err := GetUserFromContext(ctx, c.authUseCase)
	if err != nil {
		return 0, err
	}
	return user.Id, nil
}

func (c *ChatHandler) SendMessage(req *chatpb.SendMessageRequest, stream chatpb.ChatService_SendMessageServer) error {
	ctx := stream.Context()
	logger.D("SendMessage: session=%d", req.GetSessionId())
	userID, err := c.getUserID(ctx)
	if err != nil {
		return err
	}

	if len(req.Messages) == 0 {
		logger.W("SendMessage: пустой список сообщений")
		return status.Error(codes.InvalidArgument, "сообщения не предоставлены")
	}

	lastMessage := req.Messages[len(req.Messages)-1]
	lastRole := strings.ToLower(strings.TrimSpace(lastMessage.GetRole()))
	if lastRole == "assistant" {
		logger.W("SendMessage: последнее сообщение с role=assistant")
		return status.Error(codes.InvalidArgument, "последнее сообщение в запросе должно быть от пользователя (role=user)")
	}

	userMessage := lastMessage.Content
	attachmentName := ""
	if lastMessage.AttachmentName != nil {
		attachmentName = *lastMessage.AttachmentName
	}
	var attachmentContent []byte
	if lastMessage.AttachmentContent != nil {
		attachmentContent = lastMessage.AttachmentContent
	}

	responseChan, messageId, err := c.chatUseCase.SendMessage(ctx, userID, req.GetSessionId(), userMessage, attachmentName, attachmentContent)
	if err != nil {
		logger.E("SendMessage: %v", err)
		if mapped := statusForModelResolutionError(err); mapped != nil {
			return mapped
		}

		if errors.Is(err, document.ErrUnsupportedAttachmentType) || errors.Is(err, document.ErrInvalidTextEncoding) {
			return status.Error(codes.InvalidArgument, err.Error())
		}

		if strings.Contains(err.Error(), "вложение") || strings.Contains(err.Error(), "размер вложения") {
			return status.Error(codes.InvalidArgument, err.Error())
		}

		return ToStatusError(codes.Internal, err)
	}
	logger.V("SendMessage: стрим ответа запущен messageId=%d", messageId)

	createdAt := time.Now().Unix()

	for chunk := range responseChan {
		err := stream.Send(&chatpb.ChatResponse{
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

	return stream.Send(&chatpb.ChatResponse{
		Id:        messageId,
		Content:   "",
		Role:      "assistant",
		CreatedAt: createdAt,
		Done:      true,
	})
}

func (c *ChatHandler) CreateSession(ctx context.Context, req *chatpb.CreateSessionRequest) (*chatpb.ChatSession, error) {
	logger.D("CreateSession: title=%s", req.GetTitle())
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	session, err := c.chatUseCase.CreateSession(ctx, userID, req.GetTitle())
	if err != nil {
		logger.E("CreateSession: %v", err)
		return nil, ToStatusError(codes.Internal, err)
	}
	logger.I("CreateSession: создана сессия id=%d", session.Id)
	return mappers.SessionToProto(session), nil
}

func (c *ChatHandler) GetSession(ctx context.Context, req *chatpb.GetSessionRequest) (*chatpb.ChatSession, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	session, err := c.chatUseCase.GetSession(ctx, userID, req.SessionId)
	if err != nil {
		logger.W("GetSession: session=%d: %v", req.SessionId, err)
		return nil, ToStatusError(codes.NotFound, err)
	}

	return mappers.SessionToProto(session), nil
}

func (c *ChatHandler) GetSessions(ctx context.Context, req *chatpb.GetSessionsRequest) (*chatpb.GetSessionsResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	page, pageSize := normalizePagination(req.Page, req.PageSize, 20)

	sessions, total, err := c.chatUseCase.GetSessions(ctx, userID, page, pageSize)
	if err != nil {
		logger.E("GetSessions: %v", err)
		return nil, ToStatusError(codes.Internal, err)
	}

	protoSessions := make([]*chatpb.ChatSession, len(sessions))
	for i, session := range sessions {
		protoSessions[i] = mappers.SessionToProto(session)
	}

	return &chatpb.GetSessionsResponse{
		Sessions: protoSessions,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (c *ChatHandler) GetSessionMessages(ctx context.Context, req *chatpb.GetSessionMessagesRequest) (*chatpb.GetSessionMessagesResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	page, pageSize := normalizePagination(req.Page, req.PageSize, 50)

	messages, total, err := c.chatUseCase.GetSessionMessages(ctx, userID, req.SessionId, page, pageSize)
	if err != nil {
		logger.E("GetSessionMessages: %v", err)
		return nil, ToStatusError(codes.Internal, err)
	}

	protoMessages := make([]*chatpb.ChatMessage, len(messages))
	for i, msg := range messages {
		protoMessages[i] = mappers.MessageToProto(msg)
	}

	return &chatpb.GetSessionMessagesResponse{
		Messages: protoMessages,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (c *ChatHandler) DeleteSession(ctx context.Context, req *chatpb.DeleteSessionRequest) (*commonpb.Empty, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := c.chatUseCase.DeleteSession(ctx, userID, req.SessionId); err != nil {
		logger.E("DeleteSession: %v", err)
		return nil, ToStatusError(codes.Internal, err)
	}
	logger.I("DeleteSession: сессия удалена session=%d", req.SessionId)

	return &commonpb.Empty{}, nil
}

func (c *ChatHandler) UpdateSessionTitle(ctx context.Context, req *chatpb.UpdateSessionTitleRequest) (*chatpb.ChatSession, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	session, err := c.chatUseCase.UpdateSessionTitle(ctx, userID, req.SessionId, req.Title)
	if err != nil {
		logger.E("UpdateSessionTitle: %v", err)
		return nil, ToStatusError(codes.Internal, err)
	}

	return mappers.SessionToProto(session), nil
}

func mapSessionSettings(s *domain.ChatSessionSettings) *chatpb.SessionSettings {
	if s == nil {
		return &chatpb.SessionSettings{}
	}

	return &chatpb.SessionSettings{
		SessionId:      s.SessionID,
		SystemPrompt:   s.SystemPrompt,
		StopSequences:  s.StopSequences,
		TimeoutSeconds: s.TimeoutSeconds,
		Temperature:    s.Temperature,
		MaxTokens:      s.MaxTokens,
		TopK:           s.TopK,
		TopP:           s.TopP,
		JsonMode:       s.JSONMode,
		JsonSchema:     s.JSONSchema,
		ToolsJson:      s.ToolsJSON,
		Profile:        s.Profile,
	}
}

func (c *ChatHandler) GetSessionSettings(ctx context.Context, req *chatpb.GetSessionSettingsRequest) (*chatpb.SessionSettings, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	s, err := c.chatUseCase.GetSessionSettings(ctx, userID, req.GetSessionId())
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	return mapSessionSettings(s), nil
}

func (c *ChatHandler) UpdateSessionSettings(ctx context.Context, req *chatpb.UpdateSessionSettingsRequest) (*chatpb.SessionSettings, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}
	s, err := c.chatUseCase.UpdateSessionSettings(
		ctx,
		userID,
		req.GetSessionId(),
		req.GetSystemPrompt(),
		req.GetStopSequences(),
		req.GetTimeoutSeconds(),
		req.Temperature,
		req.MaxTokens,
		req.TopK,
		req.TopP,
		req.GetJsonMode(),
		req.GetJsonSchema(),
		req.GetToolsJson(),
		req.GetProfile(),
	)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	return mapSessionSettings(s), nil
}

func (c *ChatHandler) CheckConnection(ctx context.Context, req *commonpb.Empty) (*chatpb.ConnectionResponse, error) {
	return &chatpb.ConnectionResponse{IsConnected: true}, nil
}

func (c *ChatHandler) GetSelectedRunner(ctx context.Context, req *commonpb.Empty) (*chatpb.SelectedRunnerResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	runner, err := c.chatUseCase.GetSelectedRunner(ctx, userID)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	return &chatpb.SelectedRunnerResponse{Runner: runner}, nil
}

func (c *ChatHandler) SetSelectedRunner(ctx context.Context, req *chatpb.SetSelectedRunnerRequest) (*commonpb.Empty, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := c.chatUseCase.SetSelectedRunner(ctx, userID, req.GetRunner()); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	return &commonpb.Empty{}, nil
}

func (c *ChatHandler) GetDefaultRunnerModel(ctx context.Context, req *chatpb.GetDefaultRunnerModelRequest) (*chatpb.DefaultRunnerModelResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	model, err := c.chatUseCase.GetDefaultRunnerModel(ctx, userID, req.GetRunner())
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	return &chatpb.DefaultRunnerModelResponse{Model: model}, nil
}

func (c *ChatHandler) SetDefaultRunnerModel(ctx context.Context, req *chatpb.SetDefaultRunnerModelRequest) (*commonpb.Empty, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := c.chatUseCase.SetDefaultRunnerModel(ctx, userID, req.GetRunner(), req.GetModel()); err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	return &commonpb.Empty{}, nil
}
