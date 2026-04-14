package handler

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/magomedcoder/gen/api/pb/chatpb"
	"github.com/magomedcoder/gen/api/pb/commonpb"
	"github.com/magomedcoder/gen/internal/config"
	"github.com/magomedcoder/gen/internal/delivery/mappers"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/rpcmeta"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
	"github.com/magomedcoder/gen/pkg/spreadsheet"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

const maxChatEmbedBatchSize = 256

func voskTopLevelZipPaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		name := e.Name()
		if strings.Contains(name, "..") {
			continue
		}

		if !strings.HasSuffix(strings.ToLower(name), ".zip") {
			continue
		}

		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}

		names = append(names, name)
	}

	sort.Strings(names)

	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, filepath.Join(dir, n))
	}

	return out, nil
}

type ChatHandler struct {
	chatpb.UnimplementedChatServiceServer
	cfg            *config.Config
	chatUseCase    *usecase.ChatUseCase
	authUseCase    *usecase.AuthUseCase
	documentIngest *usecase.DocumentIngestUseCase
}

func NewChatHandler(cfg *config.Config, chatUseCase *usecase.ChatUseCase, authUseCase *usecase.AuthUseCase, documentIngest *usecase.DocumentIngestUseCase) *ChatHandler {
	return &ChatHandler{
		cfg:            cfg,
		chatUseCase:    chatUseCase,
		authUseCase:    authUseCase,
		documentIngest: documentIngest,
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
	ctx := rpcmeta.EnsureTraceInContext(stream.Context())
	logger.D("SendMessage: session=%d trace_id=%s", req.GetSessionId(), rpcmeta.TraceIDFromContext(ctx))
	userID, err := c.getUserID(ctx)
	if err != nil {
		return err
	}

	if req == nil || req.GetSessionId() <= 0 {
		return status.Error(codes.InvalidArgument, "некорректный session_id")
	}

	userMessage := req.GetText()
	var attachmentFileID *int64
	if fid := req.GetAttachmentFileId(); fid != 0 {
		v := fid
		attachmentFileID = &v
	}

	if strings.TrimSpace(userMessage) == "" && attachmentFileID == nil {
		logger.W("SendMessage: пустой запрос")
		return status.Error(codes.InvalidArgument, "укажите текст сообщения или attachment_file_id")
	}

	var fileRAG *usecase.SendMessageFileRAGOptions
	if req.GetUseFileRag() || req.GetFileRagTopK() != 0 || strings.TrimSpace(req.GetFileRagEmbedModel()) != "" {
		fileRAG = &usecase.SendMessageFileRAGOptions{
			UseFileRAG: req.GetUseFileRag(),
			TopK:       int(req.GetFileRagTopK()),
			EmbedModel: strings.TrimSpace(req.GetFileRagEmbedModel()),
		}
	}

	var responseChan chan usecase.ChatStreamChunk
	var sendErr error
	responseChan, sendErr = c.chatUseCase.SendMessage(ctx, userID, req.GetSessionId(), userMessage, attachmentFileID, fileRAG)
	if sendErr != nil {
		logger.E("SendMessage: %v", sendErr)
		if mapped := statusForModelResolutionError(sendErr); mapped != nil {
			return mapped
		}

		if errors.Is(sendErr, domain.ErrRAGNotConfigured) || errors.Is(sendErr, domain.ErrRAGIndexNotReady) ||
			errors.Is(sendErr, domain.ErrRAGIndexFailed) || errors.Is(sendErr, domain.ErrRAGNoHits) {
			return status.Error(codes.FailedPrecondition, sendErr.Error())
		}

		if errors.Is(sendErr, document.ErrUnsupportedAttachmentType) || errors.Is(sendErr, document.ErrInvalidTextEncoding) || errors.Is(sendErr, document.ErrTextExtractionFailed) {
			return status.Error(codes.InvalidArgument, sendErr.Error())
		}

		if strings.Contains(sendErr.Error(), "вложение") || strings.Contains(sendErr.Error(), "размер вложения") {
			return status.Error(codes.InvalidArgument, sendErr.Error())
		}

		return ToStatusError(codes.Internal, sendErr)
	}
	logger.V("SendMessage: стрим ответа запущен session=%d trace_id=%s", req.GetSessionId(), rpcmeta.TraceIDFromContext(ctx))

	createdAt := time.Now().Unix()
	var lastMsgID int64

	for chunk := range responseChan {
		if (chunk.Kind == usecase.StreamChunkKindText || chunk.Kind == usecase.StreamChunkKindReasoning) && chunk.MessageID != 0 {
			lastMsgID = chunk.MessageID
		}

		respID := chunk.MessageID
		if respID == 0 {
			respID = lastMsgID
		}

		pbKind := chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT
		if chunk.Kind == usecase.StreamChunkKindToolStatus {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TOOL_STATUS
		} else if chunk.Kind == usecase.StreamChunkKindNotice {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_NOTICE
		} else if chunk.Kind == usecase.StreamChunkKindReasoning {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_REASONING
		}
		role := "assistant"
		if chunk.Kind == usecase.StreamChunkKindNotice {
			role = "system"
		}

		resp := &chatpb.ChatResponse{
			Id:        respID,
			Content:   chunk.Text,
			Role:      role,
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
	ctx := rpcmeta.EnsureTraceInContext(stream.Context())
	logger.D("RegenerateAssistantResponse: session=%d assistantMsg=%d trace_id=%s", req.GetSessionId(), req.GetAssistantMessageId(), rpcmeta.TraceIDFromContext(ctx))
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
		if (chunk.Kind == usecase.StreamChunkKindText || chunk.Kind == usecase.StreamChunkKindReasoning) && chunk.MessageID != 0 {
			lastMsgID = chunk.MessageID
		}

		respID := chunk.MessageID
		if respID == 0 {
			respID = lastMsgID
		}

		pbKind := chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT
		if chunk.Kind == usecase.StreamChunkKindToolStatus {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TOOL_STATUS
		} else if chunk.Kind == usecase.StreamChunkKindNotice {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_NOTICE
		} else if chunk.Kind == usecase.StreamChunkKindReasoning {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_REASONING
		}
		role := "assistant"
		if chunk.Kind == usecase.StreamChunkKindNotice {
			role = "system"
		}

		resp := &chatpb.ChatResponse{
			Id:        respID,
			Content:   chunk.Text,
			Role:      role,
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

func (c *ChatHandler) ContinueAssistantResponse(req *chatpb.ContinueAssistantRequest, stream chatpb.ChatService_ContinueAssistantResponseServer) error {
	ctx := rpcmeta.EnsureTraceInContext(stream.Context())
	logger.D("ContinueAssistantResponse: session=%d assistantMsg=%d trace_id=%s", req.GetSessionId(), req.GetAssistantMessageId(), rpcmeta.TraceIDFromContext(ctx))
	userID, err := c.getUserID(ctx)
	if err != nil {
		return err
	}

	responseChan, contErr := c.chatUseCase.ContinueAssistantResponse(ctx, userID, req.GetSessionId(), req.GetAssistantMessageId())
	if contErr != nil {
		logger.E("ContinueAssistantResponse: %v", contErr)
		if mapped := statusForModelResolutionError(contErr); mapped != nil {
			return mapped
		}

		if errors.Is(contErr, domain.ErrRegenerateToolsNotSupported) {
			return status.Error(codes.FailedPrecondition, contErr.Error())
		}

		msg := contErr.Error()
		if strings.Contains(msg, "продолжить можно только") ||
			strings.Contains(msg, "нечего продолжать") ||
			strings.Contains(msg, "нет частичного") ||
			strings.Contains(msg, "не найдено") ||
			strings.Contains(msg, "некорректный assistant_message_id") ||
			strings.Contains(msg, "только ответ ассистента") {
			return status.Error(codes.InvalidArgument, msg)
		}

		if errors.Is(contErr, domain.ErrUnauthorized) {
			return status.Error(codes.PermissionDenied, contErr.Error())
		}

		return ToStatusError(codes.Internal, contErr)
	}

	createdAt := time.Now().Unix()
	var lastMsgID int64

	for chunk := range responseChan {
		if (chunk.Kind == usecase.StreamChunkKindText || chunk.Kind == usecase.StreamChunkKindReasoning) && chunk.MessageID != 0 {
			lastMsgID = chunk.MessageID
		}

		respID := chunk.MessageID
		if respID == 0 {
			respID = lastMsgID
		}

		pbKind := chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT
		if chunk.Kind == usecase.StreamChunkKindToolStatus {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TOOL_STATUS
		} else if chunk.Kind == usecase.StreamChunkKindNotice {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_NOTICE
		} else if chunk.Kind == usecase.StreamChunkKindReasoning {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_REASONING
		}
		role := "assistant"
		if chunk.Kind == usecase.StreamChunkKindNotice {
			role = "system"
		}

		resp := &chatpb.ChatResponse{
			Id:        respID,
			Content:   chunk.Text,
			Role:      role,
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
	ctx := rpcmeta.EnsureTraceInContext(stream.Context())
	logger.D("EditUserMessageAndContinue: session=%d userMsg=%d trace_id=%s", req.GetSessionId(), req.GetUserMessageId(), rpcmeta.TraceIDFromContext(ctx))
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
		if (chunk.Kind == usecase.StreamChunkKindText || chunk.Kind == usecase.StreamChunkKindReasoning) && chunk.MessageID != 0 {
			lastMsgID = chunk.MessageID
		}

		respID := chunk.MessageID
		if respID == 0 {
			respID = lastMsgID
		}

		pbKind := chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT
		if chunk.Kind == usecase.StreamChunkKindToolStatus {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TOOL_STATUS
		} else if chunk.Kind == usecase.StreamChunkKindNotice {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_NOTICE
		} else if chunk.Kind == usecase.StreamChunkKindReasoning {
			pbKind = chatpb.StreamChunkKind_STREAM_CHUNK_KIND_REASONING
		}
		role := "assistant"
		if chunk.Kind == usecase.StreamChunkKindNotice {
			role = "system"
		}

		resp := &chatpb.ChatResponse{
			Id:        respID,
			Content:   chunk.Text,
			Role:      role,
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

func (c *ChatHandler) GetAssistantMessageRegenerations(ctx context.Context, req *chatpb.GetAssistantMessageRegenerationsRequest) (*chatpb.GetAssistantMessageRegenerationsResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil || req.GetSessionId() <= 0 || req.GetAssistantMessageId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "некорректный запрос")
	}

	rows, getErr := c.chatUseCase.GetAssistantMessageRegenerations(ctx, userID, req.GetSessionId(), req.GetAssistantMessageId())
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

	out := &chatpb.GetAssistantMessageRegenerationsResponse{
		Regenerations: make([]*chatpb.AssistantMessageRegeneration, 0, len(rows)),
	}

	for _, r := range rows {
		if r == nil {
			continue
		}

		out.Regenerations = append(out.Regenerations, &chatpb.AssistantMessageRegeneration{
			Id:         r.Id,
			MessageId:  r.MessageId,
			CreatedAt:  r.CreatedAt.Unix(),
			OldContent: r.OldContent,
			NewContent: r.NewContent,
		})
	}

	return out, nil
}

func (c *ChatHandler) GetSessionMessagesForAssistantMessageVersion(ctx context.Context, req *chatpb.GetSessionMessagesForAssistantMessageVersionRequest) (*chatpb.GetSessionMessagesResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil || req.GetSessionId() <= 0 || req.GetAssistantMessageId() <= 0 || req.GetVersionIndex() < 0 {
		return nil, status.Error(codes.InvalidArgument, "некорректный запрос")
	}

	msgs, getErr := c.chatUseCase.GetSessionMessagesForAssistantMessageVersion(
		ctx,
		userID,
		req.GetSessionId(),
		req.GetAssistantMessageId(),
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
		if errors.Is(err, domain.ErrNoRunners) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}

		if errors.Is(err, domain.ErrRunnerChatModelNotConfigured) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}

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

	_, pageSize := normalizePagination(req.Page, req.PageSize, 50)
	beforeID := req.GetBeforeMessageId()

	messages, total, hasMoreOlder, err := c.chatUseCase.GetSessionMessages(ctx, userID, req.SessionId, beforeID, pageSize)
	if err != nil {
		logger.E("GetSessionMessages: %v", err)
		return nil, ToStatusError(codes.Internal, err)
	}

	protoMessages := make([]*chatpb.ChatMessage, len(messages))
	for i, msg := range messages {
		protoMessages[i] = mappers.MessageToProto(msg)
	}

	return &chatpb.GetSessionMessagesResponse{
		Messages:     protoMessages,
		Total:        total,
		Page:         0,
		PageSize:     pageSize,
		HasMoreOlder: hasMoreOlder,
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
		SessionId:             s.SessionID,
		SystemPrompt:          s.SystemPrompt,
		StopSequences:         s.StopSequences,
		TimeoutSeconds:        s.TimeoutSeconds,
		Temperature:           s.Temperature,
		TopK:                  s.TopK,
		TopP:                  s.TopP,
		JsonMode:              s.JSONMode,
		JsonSchema:            s.JSONSchema,
		ToolsJson:             s.ToolsJSON,
		Profile:               s.Profile,
		ModelReasoningEnabled: s.ModelReasoningEnabled,
		WebSearchEnabled:      s.WebSearchEnabled,
		WebSearchProvider:     s.WebSearchProvider,
		McpEnabled:            s.MCPEnabled,
		McpServerIds:          s.MCPServerIDs,
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
		req.GetModelReasoningEnabled(),
		req.GetWebSearchEnabled(),
		req.GetWebSearchProvider(),
		req.GetMcpEnabled(),
		req.GetMcpServerIds(),
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

		if errors.Is(err, document.ErrUnsupportedAttachmentType) || errors.Is(err, document.ErrInvalidTextEncoding) || errors.Is(err, document.ErrNoExtractableText) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		msg := err.Error()
		switch {
		case strings.Contains(msg, "не настроено"),
			strings.Contains(msg, "пустой файл"),
			strings.Contains(msg, "превышает"),
			strings.Contains(msg, "некорректное имя"),
			strings.Contains(msg, "квота"),
			strings.Contains(msg, "слишком много"),
			strings.Contains(msg, "проверка документа при загрузке"),
			strings.Contains(msg, "извлечённый текст слишком длинный"):
			return nil, status.Error(codes.InvalidArgument, msg)
		default:
			return nil, ToStatusError(codes.Internal, err)
		}
	}

	return &chatpb.PutSessionFileResponse{FileId: id}, nil
}

func (c *ChatHandler) IndexSessionFile(ctx context.Context, req *chatpb.IndexSessionFileRequest) (*chatpb.IndexSessionFileResponse, error) {
	if c.documentIngest == nil {
		return nil, status.Error(codes.Unavailable, "индексация документов недоступна")
	}

	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil || req.GetSessionId() <= 0 || req.GetFileId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "некорректные session_id или file_id")
	}

	if err := c.documentIngest.IndexSessionFile(ctx, userID, req.GetSessionId(), req.GetFileId(), strings.TrimSpace(req.GetEmbedModel())); err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, status.Error(codes.PermissionDenied, "нет доступа")
		}

		if mapped := statusForModelResolutionError(err); mapped != nil {
			return nil, mapped
		}

		msg := err.Error()
		if errors.Is(err, document.ErrTextExtractionFailed) || errors.Is(err, document.ErrUnsupportedAttachmentType) {
			return nil, status.Error(codes.InvalidArgument, msg)
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	return &chatpb.IndexSessionFileResponse{}, nil
}

func (c *ChatHandler) GetIngestionStatus(ctx context.Context, req *chatpb.GetIngestionStatusRequest) (*chatpb.GetIngestionStatusResponse, error) {
	if c.documentIngest == nil {
		return &chatpb.GetIngestionStatusResponse{Status: "unavailable"}, nil
	}

	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil || req.GetSessionId() <= 0 || req.GetFileId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "некорректные session_id или file_id")
	}

	idx, err := c.documentIngest.GetIngestionStatus(ctx, userID, req.GetSessionId(), req.GetFileId())
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, status.Error(codes.PermissionDenied, "нет доступа")
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	out := &chatpb.GetIngestionStatusResponse{}
	if idx == nil {
		out.Status = "none"
		return out, nil
	}

	out.Status = idx.Status
	out.LastError = idx.LastError
	out.ChunkCount = int32(idx.ChunkCount)
	out.SourceContentSha256 = idx.SourceContentSHA256
	out.PipelineVersion = idx.PipelineVersion
	out.EmbeddingModel = idx.EmbeddingModel

	return out, nil
}

func (c *ChatHandler) DeleteSessionFileIndex(ctx context.Context, req *chatpb.DeleteSessionFileIndexRequest) (*commonpb.Empty, error) {
	if c.documentIngest == nil {
		return &commonpb.Empty{}, nil
	}

	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil || req.GetSessionId() <= 0 || req.GetFileId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "некорректные session_id или file_id")
	}

	if err := c.documentIngest.DeleteSessionFileIndex(ctx, userID, req.GetSessionId(), req.GetFileId()); err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, status.Error(codes.PermissionDenied, "нет доступа")
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	return &commonpb.Empty{}, nil
}

func (c *ChatHandler) SearchSessionKnowledge(ctx context.Context, req *chatpb.SearchSessionKnowledgeRequest) (*chatpb.SearchSessionKnowledgeResponse, error) {
	if c.documentIngest == nil {
		return nil, status.Error(codes.Unavailable, "поиск по индексу недоступен")
	}

	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil || req.GetSessionId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "некорректный session_id")
	}

	q := strings.TrimSpace(req.GetQuery())
	if q == "" {
		return nil, status.Error(codes.InvalidArgument, "пустой query")
	}

	topK := int(req.GetTopK())
	if topK <= 0 {
		topK = 5
	}

	if topK > 32 {
		topK = 32
	}

	hits, err := c.documentIngest.SearchSessionKnowledge(ctx, userID, req.GetSessionId(), strings.TrimSpace(req.GetEmbedModel()), q, topK, nil)
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, status.Error(codes.PermissionDenied, "нет доступа")
		}

		if mapped := statusForModelResolutionError(err); mapped != nil {
			return nil, mapped
		}

		return nil, ToStatusError(codes.Internal, err)
	}

	out := &chatpb.SearchSessionKnowledgeResponse{
		Hits: make([]*chatpb.RagSearchHit, 0, len(hits)),
	}

	for _, h := range hits {
		out.Hits = append(out.Hits, &chatpb.RagSearchHit{
			ChunkId:    h.DocumentRAGChunk.ID,
			FileId:     h.DocumentRAGChunk.FileID,
			ChunkIndex: int32(h.DocumentRAGChunk.ChunkIndex),
			Score:      h.Score,
			Text:       h.DocumentRAGChunk.Text,
		})
	}

	return out, nil
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

		if errors.Is(err, gorm.ErrRecordNotFound) {
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

func (c *ChatHandler) DownloadVoskModel(req *chatpb.VoskModelDownloadRequest, stream chatpb.ChatService_DownloadVoskModelServer) error {
	ctx := rpcmeta.EnsureTraceInContext(stream.Context())
	if _, err := GetUserFromContext(ctx, c.authUseCase); err != nil {
		return err
	}

	modelID := strings.TrimSpace(req.GetModelId())
	dir := strings.TrimSpace(filepath.Join(c.cfg.DataDir, "vosk-models"))
	var zipPath string
	if modelID == "" {
		paths, err := voskTopLevelZipPaths(dir)
		if err != nil {
			if os.IsNotExist(err) {
				logger.W("DownloadVoskModel: нет каталога %s", dir)
				return status.Errorf(codes.NotFound, "каталог моделей Vosk не найден")
			}

			logger.E("DownloadVoskModel: read dir %s: %v", dir, err)
			return status.Errorf(codes.Internal, "не удалось прочитать каталог моделей")
		}

		if len(paths) == 0 {
			logger.W("DownloadVoskModel: в %s нет .zip в корне", dir)
			return status.Errorf(codes.NotFound, "в каталоге моделей нет файлов .zip (ожидаются только в корне, без подпапок)")
		}

		zipPath = paths[0]
		modelID = strings.TrimSuffix(filepath.Base(zipPath), filepath.Ext(zipPath))
	} else {
		if strings.Contains(modelID, "..") || strings.ContainsAny(modelID, `/\`) {
			return status.Error(codes.InvalidArgument, "некорректный model_id")
		}
		zipPath = filepath.Join(dir, modelID+".zip")
	}

	f, err := os.Open(zipPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.W("DownloadVoskModel: нет файла %s", zipPath)
			return status.Errorf(codes.NotFound, "модель %s не найдена на сервере (ожидается %s)", modelID, zipPath)
		}
		logger.E("DownloadVoskModel: open %s: %v", zipPath, err)
		return status.Errorf(codes.Internal, "не удалось открыть архив модели")
	}
	defer f.Close()

	logger.I("DownloadVoskModel: model=%s file=%s", modelID, zipPath)

	buf := make([]byte, 64*1024)
	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			if err := stream.Send(&chatpb.VoskModelChunk{Data: buf[:n]}); err != nil {
				return err
			}
		}

		if readErr == io.EOF {
			return nil
		}

		if readErr != nil {
			logger.E("DownloadVoskModel: read: %v", readErr)
			return status.Errorf(codes.Internal, "ошибка чтения архива")
		}
	}
}
