package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/magomedcoder/gen/api/pb/app/chatpb"
	"github.com/magomedcoder/gen/api/pb/app/commonpb"
	"github.com/magomedcoder/gen/internal/config"
	"github.com/magomedcoder/gen/internal/delivery/mappers"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/rpcmeta"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc/codes"
	"gorm.io/gorm"
)

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
func streamChunkKindToPB(kind usecase.StreamChunkKind) chatpb.StreamChunkKind {
	switch kind {
	case usecase.StreamChunkKindToolStatus:
		return chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TOOL_STATUS
	case usecase.StreamChunkKindNotice:
		return chatpb.StreamChunkKind_STREAM_CHUNK_KIND_NOTICE
	case usecase.StreamChunkKindReasoning:
		return chatpb.StreamChunkKind_STREAM_CHUNK_KIND_REASONING
	case usecase.StreamChunkKindRAGMeta:
		return chatpb.StreamChunkKind_STREAM_CHUNK_KIND_RAG_META
	default:
		return chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT
	}
}

func streamChunkRole(kind usecase.StreamChunkKind) string {
	if kind == usecase.StreamChunkKindNotice || kind == usecase.StreamChunkKindRAGMeta {
		return "system"
	}
	return "assistant"
}

func ragSourcesPayloadToPB(p *usecase.RAGSourcesPayload) *chatpb.RagSourcesPayload {
	if p == nil {
		return nil
	}

	out := &chatpb.RagSourcesPayload{
		Mode:                p.Mode,
		FileId:              p.FileID,
		TopK:                p.TopK,
		NeighborWindow:      p.NeighborWindow,
		DeepRagMapCalls:     p.DeepRAGMapCalls,
		DroppedByBudget:     p.DroppedByBudget,
		FullDocumentExcerpt: p.FullDocumentExcerpt,
	}

	for _, c := range p.Chunks {
		out.Chunks = append(out.Chunks, &chatpb.RagChunkPreview{
			ChunkIndex:   c.ChunkIndex,
			Score:        c.Score,
			IsNeighbor:   c.IsNeighbor,
			HeadingPath:  c.HeadingPath,
			PdfPageStart: c.PdfPageStart,
			PdfPageEnd:   c.PdfPageEnd,
			Excerpt:      c.Excerpt,
		})
	}

	return out
}

func streamSendLoop(op string, sessionID int64, traceID string, responseChan <-chan usecase.ChatStreamChunk, send func(*chatpb.ChatResponse) error) error {
	start := time.Now()
	createdAt := time.Now().Unix()
	var lastMsgID int64
	var accText, accReasoning strings.Builder
	sendCalls := 0

	for chunk := range responseChan {
		if (chunk.Kind == usecase.StreamChunkKindText || chunk.Kind == usecase.StreamChunkKindReasoning) && chunk.MessageID != 0 {
			lastMsgID = chunk.MessageID
		}

		switch chunk.Kind {
		case usecase.StreamChunkKindText:
			accText.WriteString(chunk.Text)
		case usecase.StreamChunkKindReasoning:
			accReasoning.WriteString(chunk.Text)
		}

		respID := chunk.MessageID
		if respID == 0 {
			respID = lastMsgID
		}

		resp := &chatpb.ChatResponse{
			Id:        respID,
			Content:   chunk.Text,
			Role:      streamChunkRole(chunk.Kind),
			CreatedAt: createdAt,
			Done:      false,
			ChunkKind: streamChunkKindToPB(chunk.Kind),
		}

		if chunk.ToolName != "" {
			tn := chunk.ToolName
			resp.ToolName = &tn
		}

		if chunk.RAGMode != "" {
			rm := chunk.RAGMode
			resp.RagMode = &rm
		}

		if chunk.RAGSourcesJSON != "" {
			rj := chunk.RAGSourcesJSON
			resp.RagSourcesJson = &rj
		}

		if chunk.RAGSources != nil {
			resp.RagSources = ragSourcesPayloadToPB(chunk.RAGSources)
		}

		if err := send(resp); err != nil {
			logger.W("chat stream: op=%s session_id=%d trace_id=%q phase=chunk_send_err chunk_index=%d err=%v", op, sessionID, traceID, sendCalls, err)
			return err
		}
		sendCalls++
	}

	final := &chatpb.AssistantStreamFinal{
		AssistantMessageId: lastMsgID,
		Text:               accText.String(),
		Reasoning:          accReasoning.String(),
	}

	if err := send(&chatpb.ChatResponse{
		Id:             lastMsgID,
		Content:        "",
		Role:           "assistant",
		CreatedAt:      createdAt,
		Done:           true,
		ChunkKind:      chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TEXT,
		AssistantFinal: final,
	}); err != nil {
		logger.W("chat stream: op=%s session_id=%d trace_id=%q phase=final_send_err err=%v", op, sessionID, traceID, err)
		return err
	}
	sendCalls++

	d := time.Since(start)
	logger.I("chat stream: op=%s session_id=%d trace_id=%q phase=done chunks_sent=%d assistant_msg_id=%d wall=%s text_runes=%d reasoning_runes=%d", op, sessionID, traceID, sendCalls, lastMsgID, d, utf8.RuneCountInString(accText.String()), utf8.RuneCountInString(accReasoning.String()))

	return nil
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
		return StatusErrorWithReason(codes.InvalidArgument, "CHAT_INVALID_SESSION_ID", "некорректный session_id")
	}

	userMessage := req.GetText()
	var attachmentFileID *int64
	if fid := req.GetAttachmentFileId(); fid != 0 {
		v := fid
		attachmentFileID = &v
	}
	attachmentFileIDs := make([]int64, 0, len(req.GetAttachmentFileIds())+1)
	if attachmentFileID != nil {
		attachmentFileIDs = append(attachmentFileIDs, *attachmentFileID)
	}

	for _, fid := range req.GetAttachmentFileIds() {
		duplicate := false
		for _, existing := range attachmentFileIDs {
			if existing == fid {
				duplicate = true
				break
			}
		}

		if !duplicate {
			attachmentFileIDs = append(attachmentFileIDs, fid)
		}
	}

	if strings.TrimSpace(userMessage) == "" && len(attachmentFileIDs) == 0 {
		logger.W("SendMessage: пустой запрос")
		return StatusErrorWithReason(codes.InvalidArgument, "CHAT_SEND_EMPTY_MESSAGE", "укажите текст сообщения или attachment_file_id(s)")
	}

	var fileRAG *usecase.SendMessageFileRAGOptions
	if req.GetUseFileRag() || req.GetFileRagTopK() != 0 || strings.TrimSpace(req.GetFileRagEmbedModel()) != "" {
		fileRAG = &usecase.SendMessageFileRAGOptions{
			UseFileRAG:        req.GetUseFileRag(),
			TopK:              int(req.GetFileRagTopK()),
			EmbedModel:        strings.TrimSpace(req.GetFileRagEmbedModel()),
			ForceVectorSearch: req.GetFileRagForceVector(),
		}
	}

	var responseChan chan usecase.ChatStreamChunk
	var sendErr error
	responseChan, sendErr = c.chatUseCase.SendMessage(ctx, userID, req.GetSessionId(), userMessage, attachmentFileIDs, fileRAG)
	if sendErr != nil {
		logger.E("SendMessage: %v", sendErr)
		if mapped := statusForModelResolutionError(sendErr); mapped != nil {
			return mapped
		}

		if mapped := statusForChatSendError(sendErr); mapped != nil {
			return mapped
		}

		if strings.Contains(sendErr.Error(), "вложение") || strings.Contains(sendErr.Error(), "размер вложения") {
			return StatusErrorWithReason(codes.InvalidArgument, "CHAT_SEND_INVALID_ARGUMENT", sendErr.Error())
		}
		if strings.Contains(sendErr.Error(), "attachment_file_id") ||
			strings.Contains(sendErr.Error(), "use_file_rag") ||
			strings.Contains(sendErr.Error(), "file_rag_") ||
			strings.Contains(sendErr.Error(), "слишком много вложений") {
			return StatusErrorWithReason(codes.InvalidArgument, "CHAT_SEND_INVALID_ARGUMENT", sendErr.Error())
		}

		return ToStatusError(codes.Internal, sendErr)
	}
	logger.V("SendMessage: стрим ответа запущен session=%d trace_id=%s", req.GetSessionId(), rpcmeta.TraceIDFromContext(ctx))

	return streamSendLoop("SendMessage", req.GetSessionId(), rpcmeta.TraceIDFromContext(ctx), responseChan, stream.Send)
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

		if mapped := statusForChatAssistantOpSentinel(regErr); mapped != nil {
			return mapped
		}

		if strings.Contains(regErr.Error(), "перегенерировать можно только") ||
			strings.Contains(regErr.Error(), "не является ответом") ||
			strings.Contains(regErr.Error(), "не найдено") ||
			strings.Contains(regErr.Error(), "некорректный assistant_message_id") {
			return StatusErrorWithReason(codes.InvalidArgument, "CHAT_REGENERATE_INVALID_ARGUMENT", regErr.Error())
		}

		return ToStatusError(codes.Internal, regErr)
	}

	return streamSendLoop("RegenerateAssistantResponse", req.GetSessionId(), rpcmeta.TraceIDFromContext(ctx), responseChan, stream.Send)
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

		if mapped := statusForChatAssistantOpSentinel(contErr); mapped != nil {
			return mapped
		}

		msg := contErr.Error()
		if strings.Contains(msg, "продолжить можно только") ||
			strings.Contains(msg, "нечего продолжать") ||
			strings.Contains(msg, "нет частичного") ||
			strings.Contains(msg, "не найдено") ||
			strings.Contains(msg, "некорректный assistant_message_id") ||
			strings.Contains(msg, "только ответ ассистента") {
			return StatusErrorWithReason(codes.InvalidArgument, "CHAT_CONTINUE_INVALID_ARGUMENT", msg)
		}

		return ToStatusError(codes.Internal, contErr)
	}

	return streamSendLoop("ContinueAssistantResponse", req.GetSessionId(), rpcmeta.TraceIDFromContext(ctx), responseChan, stream.Send)
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
		if mapped := statusForChatAssistantOpSentinel(editErr); mapped != nil {
			return mapped
		}
		msg := editErr.Error()
		switch {
		case strings.Contains(msg, "некорректный"),
			strings.Contains(msg, "не может быть пустым"),
			strings.Contains(msg, "не найдено"),
			strings.Contains(msg, "редактировать можно только"):
			return StatusErrorWithReason(codes.InvalidArgument, "CHAT_EDIT_INVALID_ARGUMENT", msg)
		default:
			return ToStatusError(codes.Internal, editErr)
		}
	}

	return streamSendLoop("EditUserMessageAndContinue", req.GetSessionId(), rpcmeta.TraceIDFromContext(ctx), responseChan, stream.Send)
}

func (c *ChatHandler) GetUserMessageEdits(ctx context.Context, req *chatpb.GetUserMessageEditsRequest) (*chatpb.GetUserMessageEditsResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.GetSessionId() <= 0 || req.GetUserMessageId() <= 0 {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_INVALID_REQUEST", "некорректный запрос")
	}
	rows, getErr := c.chatUseCase.GetUserMessageEdits(ctx, userID, req.GetSessionId(), req.GetUserMessageId())
	if getErr != nil {
		return nil, statusForSessionScopedGetError(getErr)
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
		return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_INVALID_REQUEST", "некорректный запрос")
	}

	msgs, getErr := c.chatUseCase.GetSessionMessagesForUserMessageVersion(
		ctx,
		userID,
		req.GetSessionId(),
		req.GetUserMessageId(),
		req.GetVersionIndex(),
	)

	if getErr != nil {
		return nil, statusForSessionScopedGetError(getErr)
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
		return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_INVALID_REQUEST", "некорректный запрос")
	}

	rows, getErr := c.chatUseCase.GetAssistantMessageRegenerations(ctx, userID, req.GetSessionId(), req.GetAssistantMessageId())
	if getErr != nil {
		return nil, statusForSessionScopedGetError(getErr)
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
		return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_INVALID_REQUEST", "некорректный запрос")
	}

	msgs, getErr := c.chatUseCase.GetSessionMessagesForAssistantMessageVersion(
		ctx,
		userID,
		req.GetSessionId(),
		req.GetAssistantMessageId(),
		req.GetVersionIndex(),
	)
	if getErr != nil {
		return nil, statusForSessionScopedGetError(getErr)
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
		if mapped := statusForCreateSessionError(err); mapped != nil {
			return nil, mapped
		}

		return nil, ToStatusError(codes.Internal, err)
	}
	logger.I("CreateSession: создана сессия id=%d", session.Id)
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

func (c *ChatHandler) PutSessionFile(ctx context.Context, req *chatpb.PutSessionFileRequest) (*chatpb.PutSessionFileResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_PUT_FILE_EMPTY_REQUEST", "пустой запрос")
	}

	if req.GetSessionId() <= 0 {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_INVALID_SESSION_ID", "некорректный session_id")
	}

	id, err := c.chatUseCase.PutSessionFile(ctx, userID, req.GetSessionId(), req.GetFilename(), req.GetContent(), req.GetTtlSeconds())
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, StatusErrorWithReason(codes.PermissionDenied, "CHAT_UNAUTHORIZED", "нет доступа к сессии")
		}

		if mapped := statusForModelResolutionError(err); mapped != nil {
			return nil, mapped
		}

		if mapped := statusForDocumentAttachmentError(err); mapped != nil {
			return nil, mapped
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
			return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_PUT_FILE_INVALID_ARGUMENT", msg)
		default:
			return nil, ToStatusError(codes.Internal, err)
		}
	}

	return &chatpb.PutSessionFileResponse{FileId: id}, nil
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
		return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_SESSION_FILE_IDS_INVALID", "некорректные session_id или file_id")
	}

	idx, err := c.documentIngest.GetIngestionStatus(ctx, userID, req.GetSessionId(), req.GetFileId())
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, StatusErrorWithReason(codes.PermissionDenied, "CHAT_UNAUTHORIZED", "нет доступа")
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

func (c *ChatHandler) GetSessionFile(ctx context.Context, req *chatpb.GetSessionFileRequest) (*chatpb.GetSessionFileResponse, error) {
	userID, err := c.getUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req == nil {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_GET_FILE_EMPTY_REQUEST", "пустой запрос")
	}

	if req.GetSessionId() <= 0 {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_INVALID_SESSION_ID", "некорректный session_id")
	}

	if req.GetFileId() <= 0 {
		return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_INVALID_FILE_ID", "некорректный file_id")
	}

	name, data, err := c.chatUseCase.GetSessionFile(ctx, userID, req.GetSessionId(), req.GetFileId())
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return nil, StatusErrorWithReason(codes.PermissionDenied, "CHAT_UNAUTHORIZED", "нет доступа к сессии")
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, StatusErrorWithReason(codes.NotFound, "CHAT_SESSION_FILE_NOT_FOUND", "файл не найден")
		}

		msg := err.Error()
		switch {
		case strings.Contains(msg, "не найден"):
			return nil, StatusErrorWithReason(codes.NotFound, "CHAT_SESSION_FILE_NOT_FOUND", msg)
		case strings.Contains(msg, "не относится"),
			strings.Contains(msg, "не принадлежит"),
			strings.Contains(msg, "истёк"),
			strings.Contains(msg, "неверный путь"),
			strings.Contains(msg, "пустой storage_path"):
			return nil, StatusErrorWithReason(codes.PermissionDenied, "CHAT_SESSION_FILE_ACCESS_DENIED", msg)
		case strings.Contains(msg, "не настроено"),
			strings.Contains(msg, "превышает"),
			strings.Contains(msg, "некорректный file_id"):
			return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_GET_FILE_INVALID_ARGUMENT", msg)
		case errors.Is(err, document.ErrUnsupportedAttachmentType) || errors.Is(err, document.ErrInvalidTextEncoding):
			if mapped := statusForDocumentAttachmentError(err); mapped != nil {
				return nil, mapped
			}
			return nil, StatusErrorWithReason(codes.InvalidArgument, "CHAT_GET_FILE_INVALID_ARGUMENT", err.Error())
		default:
			return nil, ToStatusError(codes.Internal, err)
		}
	}

	return &chatpb.GetSessionFileResponse{Filename: name, Content: data}, nil
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
				return StatusErrorWithReason(codes.NotFound, "CHAT_VOSK_MODEL_DIR_NOT_FOUND", "каталог моделей Vosk не найден")
			}

			logger.E("DownloadVoskModel: read dir %s: %v", dir, err)
			return StatusErrorWithReason(codes.Internal, "CHAT_VOSK_MODEL_DIR_READ_FAILED", "не удалось прочитать каталог моделей")
		}

		if len(paths) == 0 {
			logger.W("DownloadVoskModel: в %s нет .zip в корне", dir)
			return StatusErrorWithReason(codes.NotFound, "CHAT_VOSK_MODEL_ZIP_MISSING", "в каталоге моделей нет файлов .zip (ожидаются только в корне, без подпапок)")
		}

		zipPath = paths[0]
		modelID = strings.TrimSuffix(filepath.Base(zipPath), filepath.Ext(zipPath))
	} else {
		if strings.Contains(modelID, "..") || strings.ContainsAny(modelID, `/\`) {
			return StatusErrorWithReason(codes.InvalidArgument, "CHAT_VOSK_INVALID_MODEL_ID", "некорректный model_id")
		}
		zipPath = filepath.Join(dir, modelID+".zip")
	}

	f, err := os.Open(zipPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.W("DownloadVoskModel: нет файла %s", zipPath)
			return StatusErrorWithReason(codes.NotFound, "CHAT_VOSK_MODEL_FILE_NOT_FOUND", fmt.Sprintf("модель %s не найдена на сервере (ожидается %s)", modelID, zipPath))
		}
		logger.E("DownloadVoskModel: open %s: %v", zipPath, err)
		return StatusErrorWithReason(codes.Internal, "CHAT_VOSK_MODEL_OPEN_FAILED", "не удалось открыть архив модели")
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
			return StatusErrorWithReason(codes.Internal, "CHAT_VOSK_MODEL_READ_FAILED", "ошибка чтения архива")
		}
	}
}
