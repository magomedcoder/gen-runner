package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/magomedcoder/gen/api/pb/chatpb"
	"github.com/magomedcoder/gen/api/pb/commonpb"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mappers"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
	"github.com/magomedcoder/gen/pkg/spreadsheet"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxChatEmbedBatchSize = 256

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

	for _, m := range req.Messages {
		if m == nil || strings.TrimSpace(m.GetRole()) == "" {
			return status.Error(codes.InvalidArgument, "у каждого сообщения должна быть задана role")
		}

		if m.AttachmentContent != nil && len(m.AttachmentContent) > 0 && len(req.Messages) > 1 {
			return status.Error(codes.InvalidArgument, "вложения поддерживаются только при одном сообщении в запросе")
		}

		if m.GetAttachmentFileId() != 0 && len(req.Messages) > 1 {
			return status.Error(codes.InvalidArgument, "attachment_file_id поддерживается только при одном сообщении в запросе")
		}
	}

	lastMessage := req.Messages[len(req.Messages)-1]
	lastRole := strings.ToLower(strings.TrimSpace(lastMessage.GetRole()))
	if lastRole == "assistant" {
		logger.W("SendMessage: последнее сообщение с role=assistant")
		return status.Error(codes.InvalidArgument, "последнее сообщение должно быть role=user или role=tool")
	}

	var responseChan chan usecase.ChatStreamChunk
	var sendErr error

	useLegacySingleUser := len(req.Messages) == 1 && lastRole == "user"

	if useLegacySingleUser {
		userMessage := lastMessage.Content
		attachmentName := ""
		if lastMessage.AttachmentName != nil {
			attachmentName = *lastMessage.AttachmentName
		}
		var attachmentContent []byte
		if lastMessage.AttachmentContent != nil {
			attachmentContent = lastMessage.AttachmentContent
		}
		if lastMessage.GetAttachmentFileId() != 0 && len(attachmentContent) > 0 {
			return status.Error(codes.InvalidArgument, "нельзя одновременно передавать вложение и attachment_file_id")
		}
		var existingFileID *int64
		if fid := lastMessage.GetAttachmentFileId(); fid != 0 {
			v := fid
			existingFileID = &v
		}
		responseChan, sendErr = c.chatUseCase.SendMessage(ctx, userID, req.GetSessionId(), userMessage, attachmentName, attachmentContent, existingFileID)
	} else {
		turns := mappers.MessagesFromProto(req.Messages, req.GetSessionId())
		for _, t := range turns {
			if t != nil {
				t.Id = 0
			}
		}
		responseChan, sendErr = c.chatUseCase.SendMessageMulti(ctx, userID, req.GetSessionId(), turns)
	}
	if sendErr != nil {
		logger.E("SendMessage: %v", sendErr)
		if mapped := statusForModelResolutionError(sendErr); mapped != nil {
			return mapped
		}

		if errors.Is(sendErr, document.ErrUnsupportedAttachmentType) || errors.Is(sendErr, document.ErrInvalidTextEncoding) || errors.Is(sendErr, document.ErrTextExtractionFailed) {
			return status.Error(codes.InvalidArgument, sendErr.Error())
		}

		if strings.Contains(sendErr.Error(), "вложение") || strings.Contains(sendErr.Error(), "размер вложения") {
			return status.Error(codes.InvalidArgument, sendErr.Error())
		}

		return ToStatusError(codes.Internal, sendErr)
	}
	logger.V("SendMessage: стрим ответа запущен")

	createdAt := time.Now().Unix()
	var lastMsgID int64

	for chunk := range responseChan {
		if chunk.Kind == usecase.StreamChunkKindText && chunk.MessageID != 0 {
			lastMsgID = chunk.MessageID
		}

		respID := chunk.MessageID
		if respID == 0 {
			respID = lastMsgID
		}

		pbKind := chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT
		if chunk.Kind == usecase.StreamChunkKindToolStatus {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TOOL_STATUS
		}

		resp := &chatpb.ChatResponse{
			Id:        respID,
			Content:   chunk.Text,
			Role:      "assistant",
			CreatedAt: createdAt,
			Done:      false,
			ChunkKind: pbKind,
		}

		if chunk.ToolName != "" {
			tn := chunk.ToolName
			resp.ToolName = &tn
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}

	return stream.Send(&chatpb.ChatResponse{
		Id:        lastMsgID,
		Content:   "",
		Role:      "assistant",
		CreatedAt: createdAt,
		Done:      true,
		ChunkKind: chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT,
	})
}

func (c *ChatHandler) RegenerateAssistantResponse(req *chatpb.RegenerateAssistantRequest, stream chatpb.ChatService_RegenerateAssistantResponseServer) error {
	ctx := stream.Context()
	logger.D("RegenerateAssistantResponse: session=%d assistantMsg=%d", req.GetSessionId(), req.GetAssistantMessageId())
	userID, err := c.getUserID(ctx)
	if err != nil {
		return err
	}

	responseChan, regErr := c.chatUseCase.RegenerateAssistantResponse(ctx, userID, req.GetSessionId(), req.GetAssistantMessageId())
	if regErr != nil {
		logger.E("RegenerateAssistantResponse: %v", regErr)
		if mapped := statusForModelResolutionError(regErr); mapped != nil {
			return mapped
		}

		if errors.Is(regErr, domain.ErrRegenerateToolsNotSupported) {
			return status.Error(codes.FailedPrecondition, regErr.Error())
		}

		if strings.Contains(regErr.Error(), "перегенерировать можно только") ||
			strings.Contains(regErr.Error(), "не является ответом") ||
			strings.Contains(regErr.Error(), "не найдено") ||
			strings.Contains(regErr.Error(), "некорректный assistant_message_id") {
			return status.Error(codes.InvalidArgument, regErr.Error())
		}

		if errors.Is(regErr, domain.ErrUnauthorized) {
			return status.Error(codes.PermissionDenied, regErr.Error())
		}

		return ToStatusError(codes.Internal, regErr)
	}

	createdAt := time.Now().Unix()
	var lastMsgID int64

	for chunk := range responseChan {
		if chunk.Kind == usecase.StreamChunkKindText && chunk.MessageID != 0 {
			lastMsgID = chunk.MessageID
		}

		respID := chunk.MessageID
		if respID == 0 {
			respID = lastMsgID
		}

		pbKind := chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT
		if chunk.Kind == usecase.StreamChunkKindToolStatus {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TOOL_STATUS
		}

		resp := &chatpb.ChatResponse{
			Id:        respID,
			Content:   chunk.Text,
			Role:      "assistant",
			CreatedAt: createdAt,
			Done:      false,
			ChunkKind: pbKind,
		}

		if chunk.ToolName != "" {
			tn := chunk.ToolName
			resp.ToolName = &tn
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}

	return stream.Send(&chatpb.ChatResponse{
		Id:        lastMsgID,
		Content:   "",
		Role:      "assistant",
		CreatedAt: createdAt,
		Done:      true,
		ChunkKind: chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT,
	})
}

func (c *ChatHandler) EditUserMessageAndContinue(req *chatpb.EditUserMessageAndContinueRequest, stream chatpb.ChatService_EditUserMessageAndContinueServer) error {
	ctx := stream.Context()
	logger.D("EditUserMessageAndContinue: session=%d userMsg=%d", req.GetSessionId(), req.GetUserMessageId())
	userID, err := c.getUserID(ctx)
	if err != nil {
		return err
	}

	responseChan, editErr := c.chatUseCase.EditUserMessageAndContinue(
		ctx,
		userID,
		req.GetSessionId(),
		req.GetUserMessageId(),
		req.GetNewContent(),
	)
	if editErr != nil {
		logger.E("EditUserMessageAndContinue: %v", editErr)
		if mapped := statusForModelResolutionError(editErr); mapped != nil {
			return mapped
		}
		msg := editErr.Error()
		switch {
		case strings.Contains(msg, "некорректный"),
			strings.Contains(msg, "не может быть пустым"),
			strings.Contains(msg, "не найдено"),
			strings.Contains(msg, "редактировать можно только"):
			return status.Error(codes.InvalidArgument, msg)
		case errors.Is(editErr, domain.ErrUnauthorized):
			return status.Error(codes.PermissionDenied, editErr.Error())
		default:
			return ToStatusError(codes.Internal, editErr)
		}
	}

	createdAt := time.Now().Unix()
	var lastMsgID int64

	for chunk := range responseChan {
		if chunk.Kind == usecase.StreamChunkKindText && chunk.MessageID != 0 {
			lastMsgID = chunk.MessageID
		}

		respID := chunk.MessageID
		if respID == 0 {
			respID = lastMsgID
		}

		pbKind := chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT
		if chunk.Kind == usecase.StreamChunkKindToolStatus {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TOOL_STATUS
		}

		resp := &chatpb.ChatResponse{
			Id:        respID,
			Content:   chunk.Text,
			Role:      "assistant",
			CreatedAt: createdAt,
			Done:      false,
			ChunkKind: pbKind,
		}

		if chunk.ToolName != "" {
			tn := chunk.ToolName
			resp.ToolName = &tn
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}

	return stream.Send(&chatpb.ChatResponse{
		Id:        lastMsgID,
		Content:   "",
		Role:      "assistant",
		CreatedAt: createdAt,
		Done:      true,
		ChunkKind: chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT,
	})
}

func (c *ChatHandler) GetUserMessageEdits(ctx context.Context, req *chatpb.GetUserMessageEditsRequest) (*chatpb.GetUserMessageEditsResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.GetSessionId() <= 0 || req.GetUserMessageId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "некорректный запрос")
	}
	rows, getErr := c.chatUseCase.GetUserMessageEdits(ctx, userID, req.GetSessionId(), req.GetUserMessageId())
	if getErr != nil {
		msg := getErr.Error()
		switch {
		case errors.Is(getErr, domain.ErrUnauthorized):
			return nil, status.Error(codes.PermissionDenied, "нет доступа к сессии")
		case strings.Contains(msg, "некорректный"),
			strings.Contains(msg, "не найдено"):
			return nil, status.Error(codes.InvalidArgument, msg)
		default:
			return nil, ToStatusError(codes.Internal, getErr)
		}
	}

	out := &chatpb.GetUserMessageEditsResponse{
		Edits: make([]*chatpb.UserMessageEdit, 0, len(rows)),
	}
	for _, e := range rows {
		if e == nil {
			continue
		}
		out.Edits = append(out.Edits, &chatpb.UserMessageEdit{
			Id:         e.Id,
			MessageId:  e.MessageId,
			CreatedAt:  e.CreatedAt.Unix(),
			OldContent: e.OldContent,
			NewContent: e.NewContent,
		})
	}
	return out, nil
}

func (c *ChatHandler) GetSessionMessagesForUserMessageVersion(ctx context.Context, req *chatpb.GetSessionMessagesForUserMessageVersionRequest) (*chatpb.GetSessionMessagesResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil || req.GetSessionId() <= 0 || req.GetUserMessageId() <= 0 || req.GetVersionIndex() < 0 {
		return nil, status.Error(codes.InvalidArgument, "некорректный запрос")
	}

	msgs, getErr := c.chatUseCase.GetSessionMessagesForUserMessageVersion(
		ctx,
		userID,
		req.GetSessionId(),
		req.GetUserMessageId(),
		req.GetVersionIndex(),
	)

	if getErr != nil {
		msg := getErr.Error()
		switch {
		case errors.Is(getErr, domain.ErrUnauthorized):
			return nil, status.Error(codes.PermissionDenied, "нет доступа к сессии")
		case strings.Contains(msg, "некорректный"),
			strings.Contains(msg, "не найдено"):
			return nil, status.Error(codes.InvalidArgument, msg)
		default:
			return nil, ToStatusError(codes.Internal, getErr)
		}
	}

	out := make([]*chatpb.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, mappers.MessageToProto(m))
	}

	return &chatpb.GetSessionMessagesResponse{
		Messages: out,
		Total:    int32(len(out)),
		Page:     1,
		PageSize: int32(len(out)),
	}, nil
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

func (c *ChatHandler) Embed(ctx context.Context, req *chatpb.EmbedRequest) (*chatpb.EmbedResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}
	text := strings.TrimSpace(req.GetText())
	if text == "" {
		return nil, status.Error(codes.InvalidArgument, "text не может быть пустым")
	}

	vec, err := c.chatUseCase.Embed(ctx, userID, req.GetModel(), text)
	if err != nil {
		if mapped := statusForModelResolutionError(err); mapped != nil {
			return nil, mapped
		}
		return nil, ToStatusError(codes.Internal, err)
	}

	return &chatpb.EmbedResponse{Values: vec}, nil
}

func (c *ChatHandler) EmbedBatch(ctx context.Context, req *chatpb.EmbedBatchRequest) (*chatpb.EmbedBatchResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}
	texts := req.GetTexts()
	if len(texts) == 0 {
		return nil, status.Error(codes.InvalidArgument, "texts не может быть пустым")
	}
	if len(texts) > maxChatEmbedBatchSize {
		return nil, status.Errorf(codes.InvalidArgument, "не более %d текстов за один запрос", maxChatEmbedBatchSize)
	}
	for i, t := range texts {
		if strings.TrimSpace(t) == "" {
			return nil, status.Errorf(codes.InvalidArgument, "texts[%d]: пустая строка", i)
		}
	}

	rows, err := c.chatUseCase.EmbedBatch(ctx, userID, req.GetModel(), texts)
	if err != nil {
		if mapped := statusForModelResolutionError(err); mapped != nil {
			return nil, mapped
		}
		return nil, ToStatusError(codes.Internal, err)
	}

	out := &chatpb.EmbedBatchResponse{
		Embeddings: make([]*chatpb.Embedding, 0, len(rows)),
	}
	for _, row := range rows {
		out.Embeddings = append(out.Embeddings, &chatpb.Embedding{Values: row})
	}
	return out, nil
}

func (c *ChatHandler) PutSessionFile(ctx context.Context, req *chatpb.PutSessionFileRequest) (*chatpb.PutSessionFileResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}

	if req.GetSessionId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "некорректный session_id")
	}

	id, err := c.chatUseCase.PutSessionFile(ctx, userID, req.GetSessionId(), req.GetFilename(), req.GetContent(), req.GetTtlSeconds())
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, status.Error(codes.PermissionDenied, "нет доступа к сессии")
		}

		if mapped := statusForModelResolutionError(err); mapped != nil {
			return nil, mapped
		}

		if errors.Is(err, document.ErrUnsupportedAttachmentType) || errors.Is(err, document.ErrInvalidTextEncoding) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		msg := err.Error()
		switch {
		case strings.Contains(msg, "не настроено"),
			strings.Contains(msg, "пустой файл"),
			strings.Contains(msg, "превышает"),
			strings.Contains(msg, "некорректное имя"),
			strings.Contains(msg, "квота"),
			strings.Contains(msg, "слишком много"):
			return nil, status.Error(codes.InvalidArgument, msg)
		default:
			return nil, ToStatusError(codes.Internal, err)
		}
	}

	return &chatpb.PutSessionFileResponse{FileId: id}, nil
}

func (c *ChatHandler) GetSessionFile(ctx context.Context, req *chatpb.GetSessionFileRequest) (*chatpb.GetSessionFileResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}

	if req.GetSessionId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "некорректный session_id")
	}

	if req.GetFileId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "некорректный file_id")
	}

	name, data, err := c.chatUseCase.GetSessionFile(ctx, userID, req.GetSessionId(), req.GetFileId())
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, status.Error(codes.PermissionDenied, "нет доступа к сессии")
		}

		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "файл не найден")
		}

		msg := err.Error()
		switch {
		case strings.Contains(msg, "не найден"):
			return nil, status.Error(codes.NotFound, msg)
		case strings.Contains(msg, "не относится"),
			strings.Contains(msg, "не принадлежит"),
			strings.Contains(msg, "истёк"),
			strings.Contains(msg, "неверный путь"),
			strings.Contains(msg, "пустой storage_path"):
			return nil, status.Error(codes.PermissionDenied, msg)
		case strings.Contains(msg, "не настроено"),
			strings.Contains(msg, "превышает"),
			strings.Contains(msg, "некорректный file_id"):
			return nil, status.Error(codes.InvalidArgument, msg)
		case errors.Is(err, document.ErrUnsupportedAttachmentType) || errors.Is(err, document.ErrInvalidTextEncoding):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		default:
			return nil, ToStatusError(codes.Internal, err)
		}
	}

	return &chatpb.GetSessionFileResponse{Filename: name, Content: data}, nil
}

func (c *ChatHandler) ApplySpreadsheet(ctx context.Context, req *chatpb.SpreadsheetApplyRequest) (*chatpb.SpreadsheetApplyResponse, error) {
	_, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}

	out, preview, exportedCSV, err := c.chatUseCase.ApplySpreadsheet(
		ctx,
		req.GetWorkbookXlsx(),
		req.GetOperationsJson(),
		req.GetPreviewSheet(),
		req.GetPreviewRange(),
	)

	if err != nil {
		if errors.Is(err, spreadsheet.ErrInvalidOp) || errors.Is(err, spreadsheet.ErrWorkbookTooLarge) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	return &chatpb.SpreadsheetApplyResponse{
		WorkbookXlsx: out,
		PreviewTsv:   preview,
		ExportedCsv:  exportedCSV,
	}, nil
}

func (c *ChatHandler) BuildDocx(ctx context.Context, req *chatpb.DocxBuildRequest) (*chatpb.DocxBuildResponse, error) {
	_, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}

	out, err := c.chatUseCase.BuildDocx(ctx, req.GetSpecJson())
	if err != nil {
		if errors.Is(err, document.ErrDocxBuildInvalidSpec) || errors.Is(err, document.ErrDocxBuildTooLarge) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	return &chatpb.DocxBuildResponse{Docx: out}, nil
}

func (c *ChatHandler) ApplyMarkdownPatch(ctx context.Context, req *chatpb.MarkdownPatchRequest) (*chatpb.MarkdownPatchResponse, error) {
	_, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "пустой запрос")
	}

	out, err := c.chatUseCase.ApplyMarkdownPatch(ctx, req.GetBaseText(), req.GetPatchJson())
	if err != nil {
		if errors.Is(err, document.ErrMdPatchInvalidSpec) || errors.Is(err, document.ErrMdPatchAmbiguousSubstr) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	return &chatpb.MarkdownPatchResponse{Text: out}, nil
}
