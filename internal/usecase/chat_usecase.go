package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mcpclient"
	"github.com/magomedcoder/gen/internal/service"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
	"github.com/magomedcoder/gen/pkg/spreadsheet"
	"github.com/magomedcoder/gen/pkg/websearch"
	"golang.org/x/sync/errgroup"
)

const defaultResponseLanguagePrompt = "Язык ответа: отвечай на том же языке, что и последнее сообщение пользователя в этом запросе. Если язык нельзя определить (например, только код, числа или нейтральные символы), отвечай по-русски."
const maxFileExtractedTextCacheBytes = 2 << 20

const (
	defaultFileRAGTopK            = 5
	maxFileRAGTopK                = 32
	maxFileRAGContextRunesCeiling = 12_000
)

type SendMessageFileRAGOptions struct {
	UseFileRAG bool
	TopK       int
	EmbedModel string
}

func forwardLLMStreamChunks(
	ctx context.Context,
	out chan<- ChatStreamChunk,
	messageID int64,
	in <-chan domain.LLMStreamChunk,
	intoContent *strings.Builder,
) {
	for chunk := range in {
		if chunk.ReasoningContent != "" {
			select {
			case <-ctx.Done():
				return
			case out <- ChatStreamChunk{Kind: StreamChunkKindReasoning, Text: chunk.ReasoningContent, MessageID: messageID}:
			}
		}

		if chunk.Content != "" {
			intoContent.WriteString(chunk.Content)
			select {
			case <-ctx.Done():
				return
			case out <- ChatStreamChunk{Kind: StreamChunkKindText, Text: chunk.Content, MessageID: messageID}:
			}
		}
	}
}

func normalizeAttachmentHydrateParallelism(n int) int {
	if n <= 0 {
		return 8
	}

	if n > 64 {
		return 64
	}

	return n
}

func chatSessionSystemMessage(sessionID int64, settings *domain.ChatSessionSettings) *domain.Message {
	var extra string
	if settings != nil {
		extra = strings.TrimSpace(settings.SystemPrompt)
	}

	text := defaultResponseLanguagePrompt
	if extra != "" {
		text = defaultResponseLanguagePrompt + "\n\n" + extra
	}

	return domain.NewMessage(sessionID, text, domain.MessageRoleSystem)
}

func (c *ChatUseCase) llmChatSystemMessage(ctx context.Context, sessionID int64, settings *domain.ChatSessionSettings, userID int) *domain.Message {
	msg := chatSessionSystemMessage(sessionID, settings)
	c.appendMCPLLMContext(ctx, msg, settings, userID)
	return msg
}

type ChatUseCase struct {
	chatTx                       domain.ChatTransactionRunner
	sessionRepo                  domain.ChatSessionRepository
	preferenceRepo               domain.ChatPreferenceRepository
	sessionSettingsRepo          domain.ChatSessionSettingsRepository
	messageRepo                  domain.MessageRepository
	messageEditRepo              domain.MessageEditRepository
	assistantRegenRepo           domain.AssistantMessageRegenerationRepository
	fileRepo                     domain.FileRepository
	runnerRepo                   domain.RunnerRepository
	llmRepo                      domain.LLMRepository
	runnerPool                   *service.Pool
	runnerReg                    *service.Registry
	attachmentsSaveDir           string
	historySummaryCache          *historySummaryCache
	attachmentHydrateParallelism int
	webSearchSettingsRepo        domain.WebSearchSettingsRepository
	mcpServerRepo                domain.MCPServerRepository
	mcpToolsListCache            *mcpclient.ToolsListCache
	documentIngest               *DocumentIngestUseCase
	ragBackgroundIndexTimeout    time.Duration
	llmContextFallbackTokens     int
	ragMaxExtractedRunesOnUpload int
}

func NewChatUseCase(
	chatTx domain.ChatTransactionRunner,
	sessionRepo domain.ChatSessionRepository,
	preferenceRepo domain.ChatPreferenceRepository,
	sessionSettingsRepo domain.ChatSessionSettingsRepository,
	messageRepo domain.MessageRepository,
	messageEditRepo domain.MessageEditRepository,
	assistantRegenRepo domain.AssistantMessageRegenerationRepository,
	fileRepo domain.FileRepository,
	runnerRepo domain.RunnerRepository,
	llmRepo domain.LLMRepository,
	runnerPool *service.Pool,
	runnerReg *service.Registry,
	attachmentsSaveDir string,
	attachmentHydrateParallelism int,
	webSearchSettingsRepo domain.WebSearchSettingsRepository,
	mcpServerRepo domain.MCPServerRepository,
	mcpToolsListCache *mcpclient.ToolsListCache,
	documentIngest *DocumentIngestUseCase,
	ragBackgroundIndexTimeout time.Duration,
	llmContextFallbackTokens int,
	ragMaxExtractedRunesOnUpload int,
) *ChatUseCase {
	if ragBackgroundIndexTimeout <= 0 {
		ragBackgroundIndexTimeout = 30 * time.Minute
	}
	return &ChatUseCase{
		chatTx:                       chatTx,
		sessionRepo:                  sessionRepo,
		preferenceRepo:               preferenceRepo,
		sessionSettingsRepo:          sessionSettingsRepo,
		messageRepo:                  messageRepo,
		messageEditRepo:              messageEditRepo,
		assistantRegenRepo:           assistantRegenRepo,
		fileRepo:                     fileRepo,
		runnerRepo:                   runnerRepo,
		llmRepo:                      llmRepo,
		runnerPool:                   runnerPool,
		runnerReg:                    runnerReg,
		attachmentsSaveDir:           attachmentsSaveDir,
		historySummaryCache:          newHistorySummaryCache(512),
		webSearchSettingsRepo:        webSearchSettingsRepo,
		mcpServerRepo:                mcpServerRepo,
		mcpToolsListCache:            mcpToolsListCache,
		documentIngest:               documentIngest,
		ragBackgroundIndexTimeout:    ragBackgroundIndexTimeout,
		llmContextFallbackTokens:     llmContextFallbackTokens,
		ragMaxExtractedRunesOnUpload: ragMaxExtractedRunesOnUpload,
		attachmentHydrateParallelism: normalizeAttachmentHydrateParallelism(attachmentHydrateParallelism),
	}
}

func normalizeWebSearchProvider(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "brave", "google", "yandex", "multi":
		return s
	default:
		return ""
	}
}

func (c *ChatUseCase) webSearcherFor(ctx context.Context, settings *domain.ChatSessionSettings) websearch.Searcher {
	g, err := c.webSearchSettingsRepo.Get(ctx)
	if err != nil || g == nil || !g.Enabled {
		return nil
	}
	p := ""
	if settings != nil {
		p = normalizeWebSearchProvider(settings.WebSearchProvider)
	}
	if p == "" {
		return nil
	}
	o := websearch.Options{
		Enabled:              true,
		Provider:             p,
		BraveAPIKey:          g.BraveAPIKey,
		GoogleAPIKey:         g.GoogleAPIKey,
		GoogleSearchEngineID: g.GoogleSearchEngineID,
		YandexUser:           g.YandexUser,
		YandexKey:            g.YandexKey,
		MaxResults:           g.MaxResults,
		YandexEnabled:        g.YandexEnabled,
		GoogleEnabled:        g.GoogleEnabled,
		BraveEnabled:         g.BraveEnabled,
	}
	if o.MaxResults <= 0 {
		o.MaxResults = 20
	}
	return websearch.New(o)
}

func (c *ChatUseCase) GetSelectedRunner(ctx context.Context, userID int) (string, error) {
	s, err := c.preferenceRepo.GetSelectedRunner(ctx, userID)
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	if s != "" {
		return s, nil
	}
	if c.runnerReg != nil {
		if a := c.runnerReg.DefaultRunnerListenAddress(); a != "" {
			return a, nil
		}
	}
	return "", nil
}

func (c *ChatUseCase) SetSelectedRunner(ctx context.Context, userID int, runner string) error {
	return c.preferenceRepo.SetSelectedRunner(ctx, userID, runner)
}

func (c *ChatUseCase) verifySessionOwnership(ctx context.Context, userId int, sessionID int64) (*domain.ChatSession, error) {
	session, err := c.sessionRepo.GetById(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if session.UserId != userId {
		return nil, domain.ErrUnauthorized
	}

	return session, nil
}

func (c *ChatUseCase) chatRunnerAddrAndModel(ctx context.Context, session *domain.ChatSession) (addr string, model string, err error) {
	if session == nil || session.SelectedRunnerID == nil || *session.SelectedRunnerID <= 0 {
		return "", "", fmt.Errorf("сессии не назначен раннер")
	}
	ru, err := c.runnerRepo.GetByID(ctx, *session.SelectedRunnerID)
	if err != nil {
		return "", "", err
	}
	if !ru.Enabled {
		return "", "", fmt.Errorf("раннер для этого чата отключён")
	}
	model = strings.TrimSpace(ru.SelectedModel)
	if model == "" {
		return "", "", fmt.Errorf("у раннера чата не задана модель")
	}
	addr = domain.RunnerListenAddress(ru.Host, ru.Port)
	if addr == "" {
		return "", "", fmt.Errorf("некорректный адрес раннера")
	}
	return addr, model, nil
}

func (c *ChatUseCase) GetModels(ctx context.Context) ([]string, error) {
	return c.llmRepo.GetModels(ctx)
}

func (c *ChatUseCase) Embed(ctx context.Context, userID int, requestedModel string, text string) ([]float32, error) {
	model, err := resolveModelForUser(ctx, c.llmRepo, strings.TrimSpace(requestedModel), "")
	if err != nil {
		return nil, err
	}

	return c.llmRepo.Embed(ctx, model, text)
}

func (c *ChatUseCase) EmbedBatch(ctx context.Context, userID int, requestedModel string, texts []string) ([][]float32, error) {
	model, err := resolveModelForUser(ctx, c.llmRepo, strings.TrimSpace(requestedModel), "")
	if err != nil {
		return nil, err
	}

	return c.llmRepo.EmbedBatch(ctx, model, texts)
}

func genParamsFromSessionSettings(settings *domain.ChatSessionSettings) (stopSequences []string, timeoutSeconds int32, genParams *domain.GenerationParams) {
	if settings == nil {
		return nil, 0, nil
	}

	stopSequences = settings.StopSequences
	timeoutSeconds = settings.TimeoutSeconds
	et := settings.ModelReasoningEnabled
	genParams = &domain.GenerationParams{
		Temperature:    settings.Temperature,
		TopK:           settings.TopK,
		TopP:           settings.TopP,
		EnableThinking: &et,
	}

	if settings.JSONMode {
		jsonSchema := strings.TrimSpace(settings.JSONSchema)
		var schemaPtr *string
		if jsonSchema != "" {
			schemaPtr = &jsonSchema
		}

		genParams.ResponseFormat = &domain.ResponseFormat{
			Type:   "json_object",
			Schema: schemaPtr,
		}
	}

	if parsedTools := parseToolsJSON(settings.ToolsJSON); len(parsedTools) > 0 {
		genParams.Tools = parsedTools
	}

	return stopSequences, timeoutSeconds, genParams
}

func (c *ChatUseCase) maybeInjectWebSearchTool(ctx context.Context, genParams *domain.GenerationParams, settings *domain.ChatSessionSettings) {
	if genParams == nil || settings == nil || !settings.WebSearchEnabled {
		return
	}
	if c.webSearcherFor(ctx, settings) == nil {
		return
	}
	for _, t := range genParams.Tools {
		if normalizeToolName(t.Name) == "web_search" {
			return
		}
	}
	genParams.Tools = append(genParams.Tools, webSearchToolDefinition())
}

func (c *ChatUseCase) maybeInjectMCPTools(ctx context.Context, genParams *domain.GenerationParams, settings *domain.ChatSessionSettings, userID int) {
	if genParams == nil || settings == nil || !settings.MCPEnabled || len(settings.MCPServerIDs) == 0 || c.mcpServerRepo == nil {
		return
	}

	allowed := allowedToolNameSet(genParams.Tools)
	for _, sid := range settings.MCPServerIDs {
		if sid <= 0 {
			continue
		}

		srv, err := c.mcpServerRepo.GetByIDAccessible(ctx, sid, userID)
		if err != nil || srv == nil || !srv.Enabled {
			if err != nil {
				logger.W("MCP: сервер id=%d: %v", sid, err)
			}
			continue
		}

		declared, err := c.mcpToolsListCache.ListToolsCached(ctx, srv, mcpclient.DefaultToolsListCacheTTL)
		if err != nil {
			logger.W("MCP: tools/list id=%d: %v", sid, err)
			continue
		}

		prefix := strings.TrimSpace(srv.Name)
		for _, t := range declared {
			if strings.TrimSpace(t.Name) == "" {
				continue
			}
			alias := mcpclient.ToolAlias(sid, t.Name)
			n := normalizeToolName(alias)
			if _, dup := allowed[n]; dup {
				continue
			}

			allowed[n] = struct{}{}
			desc := strings.TrimSpace(t.Description)
			if prefix != "" {
				desc = "[MCP " + prefix + "] " + desc
			} else {
				desc = "[MCP #" + strconv.FormatInt(sid, 10) + "] " + desc
			}

			genParams.Tools = append(genParams.Tools, domain.Tool{
				Name:           alias,
				Description:    strings.TrimSpace(desc),
				ParametersJSON: t.ParametersJSON,
			})
		}
	}
}

func (c *ChatUseCase) toolMCP(ctx context.Context, sessionID int64, serverID int64, mcpToolName string, params json.RawMessage) (string, error) {
	if c.mcpServerRepo == nil {
		return "", fmt.Errorf("MCP недоступен")
	}

	settings, err := c.sessionSettingsRepo.GetBySessionID(ctx, sessionID)
	if err != nil {
		return "", err
	}

	if settings == nil || !settings.MCPEnabled {
		return "", fmt.Errorf("MCP отключён для этой сессии")
	}

	allowed := false
	for _, id := range settings.MCPServerIDs {
		if id == serverID {
			allowed = true
			break
		}
	}

	if !allowed {
		return "", fmt.Errorf("этот MCP-сервер не выбран для сессии")
	}

	sess, err := c.sessionRepo.GetById(ctx, sessionID)
	if err != nil || sess == nil {
		return "", fmt.Errorf("сессия не найдена")
	}

	srv, err := c.mcpServerRepo.GetByIDAccessible(ctx, serverID, sess.UserId)
	if err != nil {
		return "", err
	}

	if srv == nil || !srv.Enabled {
		return "", fmt.Errorf("MCP-сервер недоступен")
	}

	toolCtx := ctx
	if env := toolLoopEnvFrom(ctx); env != nil && mcpclient.SamplingEnabled() {
		toolCtx = mcpclient.WithSamplingRunner(ctx, &mcpclient.SamplingRunner{
			LLM:            c.llmRepo,
			SessionID:      sessionID,
			RunnerAddr:     env.RunnerAddr,
			Model:          env.ResolvedModel,
			StopSequences:  env.StopSequences,
			TimeoutSeconds: env.TimeoutSeconds,
			GenParams:      env.SamplingGen,
		})
	}

	return runFnWithContext(ctx, func() (string, error) {
		t0 := time.Now()
		out, err := mcpclient.CallTool(toolCtx, srv, mcpToolName, params, c.mcpToolsListCache)
		d := time.Since(t0)
		if err != nil {
			logger.W("MCP tools/call: session=%d server_id=%d tool=%q duration=%s err=%v", sessionID, serverID, mcpToolName, d, err)
		} else {
			logger.I("MCP tools/call: session=%d server_id=%d tool=%q duration=%s", sessionID, serverID, mcpToolName, d)
		}

		return out, err
	})
}

func (c *ChatUseCase) SendMessage(ctx context.Context, userId int, sessionId int64, userMessage string, attachmentFileID *int64, fileRAG *SendMessageFileRAGOptions) (chan ChatStreamChunk, error) {
	logger.D("SendMessage: session=%d user=%d", sessionId, userId)
	session, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		logger.W("SendMessage: сессия не принадлежит пользователю: %v", err)
		return nil, err
	}

	runnerAddr, resolvedModel, err := c.chatRunnerAddrAndModel(ctx, session)
	if err != nil {
		logger.W("SendMessage: раннер/модель: %v", err)
		return nil, err
	}

	messages, err := c.historyMessagesForLLM(ctx, sessionId)
	if err != nil {
		logger.E("SendMessage: история для LLM: %v", err)
		return nil, err
	}

	var attachmentName string
	var attachmentContent []byte
	var storedAttachmentFileID *int64

	if attachmentFileID != nil && *attachmentFileID > 0 {
		name, content, err := c.loadSessionAttachmentForSend(ctx, userId, sessionId, *attachmentFileID)
		if err != nil {
			return nil, err
		}
		attachmentName = name
		attachmentContent = content
		storedAttachmentFileID = attachmentFileID
	}

	if fileRAG != nil && fileRAG.UseFileRAG && (attachmentFileID == nil || *attachmentFileID <= 0) {
		return nil, fmt.Errorf("режим use_file_rag требует attachment_file_id")
	}

	userMsg := domain.NewMessageWithAttachment(sessionId, userMessage, domain.MessageRoleUser, storedAttachmentFileID)
	if err := c.messageRepo.Create(ctx, userMsg); err != nil {
		logger.E("SendMessage: создание сообщения: %v", err)
		return nil, err
	}

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	messagesForLLM := make([]*domain.Message, 0, len(messages)+2)
	messagesForLLM = append(messagesForLLM, c.llmChatSystemMessage(ctx, sessionId, settings, userId))
	messagesForLLM = append(messagesForLLM, messages...)
	if len(attachmentContent) > 0 && attachmentName != "" {
		userMsgForLLM := *userMsg
		if document.IsImageAttachment(attachmentName) {
			userMsgForLLM.Content = userMessage
			userMsgForLLM.AttachmentName = attachmentName
			userMsgForLLM.AttachmentContent = attachmentContent
		} else if fileRAG != nil && fileRAG.UseFileRAG {
			if c.documentIngest == nil {
				return nil, domain.ErrRAGNotConfigured
			}

			fid := *storedAttachmentFileID
			idx, err := c.documentIngest.GetIngestionStatus(ctx, userId, sessionId, fid)
			if err != nil {
				return nil, err
			}

			if idx != nil && idx.Status == domain.FileRAGIndexStatusFailed {
				msg := strings.TrimSpace(idx.LastError)
				if msg == "" {
					msg = "см. журнал сервера"
				}

				return nil, fmt.Errorf("%w: %s", domain.ErrRAGIndexFailed, msg)
			}

			if idx == nil || idx.Status != domain.FileRAGIndexStatusReady {
				st := "нет записи"
				if idx != nil {
					st = idx.Status
				}

				return nil, fmt.Errorf("%w: статус=%s", domain.ErrRAGIndexNotReady, st)
			}

			topK := fileRAG.TopK
			if topK <= 0 {
				topK = defaultFileRAGTopK
			}

			if topK > maxFileRAGTopK {
				topK = maxFileRAGTopK
			}

			query := strings.TrimSpace(userMessage)
			if query == "" {
				query = "Ответь по содержимому прикреплённого документа."
			}

			scored, err := c.documentIngest.SearchSessionKnowledge(ctx, userId, sessionId, fileRAG.EmbedModel, query, topK, &fid)
			if err != nil {
				return nil, err
			}

			if len(scored) == 0 {
				return nil, domain.ErrRAGNoHits
			}

			ragRunes := c.effectiveMaxRAGContextRunes(messagesForLLM, userMessage)
			userMsgForLLM.Content = buildMessageWithRAG(attachmentName, userMessage, scored, ragRunes)
		} else {
			built, err := buildMessageWithFile(attachmentName, attachmentContent, userMessage)
			if err != nil {
				return nil, err
			}
			userMsgForLLM.Content = built
		}
		messagesForLLM = append(messagesForLLM, &userMsgForLLM)
	} else {
		messagesForLLM = append(messagesForLLM, userMsg)
	}

	stopSequences, timeoutSeconds, genParams := genParamsFromSessionSettings(settings)
	c.maybeInjectWebSearchTool(ctx, genParams, settings)
	c.maybeInjectMCPTools(ctx, genParams, settings, userId)
	c.maybeInjectMCPBuiltinMetaTools(genParams, settings)

	if err := c.hydrateAttachmentsForRunner(ctx, messagesForLLM); err != nil {
		logger.E("SendMessage: подгрузка вложений для раннера: %v", err)
		return nil, err
	}
	var historyNotice bool
	messagesForLLM, historyNotice = c.capLLMHistoryTokens(ctx, messagesForLLM, 1, sessionId, resolvedModel, runnerAddr, true)

	if genParams != nil && len(genParams.Tools) > 0 {
		return c.sendMessageWithToolLoop(ctx, userId, sessionId, runnerAddr, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams, historyNotice)
	}

	assistantMsg := domain.NewMessage(sessionId, "", domain.MessageRoleAssistant)
	if err := c.messageRepo.Create(ctx, assistantMsg); err != nil {
		logger.E("SendMessage: создание черновика ответа: %v", err)
		return nil, err
	}
	messageID := assistantMsg.Id

	responseChan, err := c.llmRepo.SendMessageOnRunner(ctx, runnerAddr, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
	if err != nil {
		logger.E("SendMessage: вызов LLM: %v", err)
		return nil, err
	}
	logger.V("SendMessage: стрим от LLM запущен session=%d", sessionId)

	var fullResponse strings.Builder
	clientChan := make(chan ChatStreamChunk, 100)
	go func() {
		defer func() {
			_ = c.messageRepo.UpdateContent(context.Background(), messageID, fullResponse.String())
		}()
		defer close(clientChan)

		if historyNotice {
			select {
			case <-ctx.Done():
				return
			case clientChan <- ChatStreamChunk{Kind: StreamChunkKindNotice, Text: HistoryTruncatedClientNotice}:
			}
		}

		forwardLLMStreamChunks(ctx, clientChan, messageID, responseChan, &fullResponse)
	}()

	return clientChan, nil
}

func (c *ChatUseCase) RegenerateAssistantResponse(ctx context.Context, userId int, sessionId int64, assistantMessageID int64) (chan ChatStreamChunk, error) {
	logger.D("RegenerateAssistantResponse: session=%d user=%d assistantMsg=%d", sessionId, userId, assistantMessageID)
	if assistantMessageID <= 0 {
		return nil, fmt.Errorf("некорректный assistant_message_id")
	}

	session, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		logger.W("RegenerateAssistantResponse: сессия: %v", err)
		return nil, err
	}
	runnerAddr, resolvedModel, err := c.chatRunnerAddrAndModel(ctx, session)
	if err != nil {
		return nil, err
	}

	target, err := c.messageRepo.GetByID(ctx, assistantMessageID)
	if err != nil {
		logger.E("RegenerateAssistantResponse: загрузка сообщения: %v", err)
		return nil, err
	}
	if target == nil || target.SessionId != sessionId {
		return nil, fmt.Errorf("сообщение не найдено")
	}
	if target.Role != domain.MessageRoleAssistant {
		return nil, fmt.Errorf("перегенерировать можно только ответ ассистента")
	}
	oldContent := target.Content

	maxID, err := c.messageRepo.MaxMessageIDInSession(ctx, sessionId)
	if err != nil {
		logger.E("RegenerateAssistantResponse: max id: %v", err)
		return nil, err
	}
	if maxID != assistantMessageID {
		return nil, fmt.Errorf("перегенерировать можно только последнее сообщение в чате")
	}

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	if len(parseToolsJSON(settings.ToolsJSON)) > 0 {
		return nil, domain.ErrRegenerateToolsNotSupported
	}
	if settings.MCPEnabled && len(settings.MCPServerIDs) > 0 {
		return nil, domain.ErrRegenerateToolsNotSupported
	}
	stopSequences, timeoutSeconds, genParams := genParamsFromSessionSettings(settings)

	rawPrefix, err := c.messageRepo.ListMessagesWithIDLessThan(ctx, sessionId, assistantMessageID)
	if err != nil {
		logger.E("RegenerateAssistantResponse: префикс истории: %v", err)
		return nil, err
	}
	messages := filterHistoryForLLM(rawPrefix)

	messagesForLLM := make([]*domain.Message, 0, len(messages)+1)
	messagesForLLM = append(messagesForLLM, chatSessionSystemMessage(sessionId, settings))
	messagesForLLM = append(messagesForLLM, messages...)

	if err := c.hydrateAttachmentsForRunner(ctx, messagesForLLM); err != nil {
		logger.E("RegenerateAssistantResponse: вложения: %v", err)
		return nil, err
	}
	var regenHistoryNotice bool
	messagesForLLM, regenHistoryNotice = c.capLLMHistoryTokens(ctx, messagesForLLM, 1, sessionId, resolvedModel, runnerAddr, true)

	if err := c.messageRepo.ResetAssistantForRegenerate(ctx, sessionId, assistantMessageID); err != nil {
		logger.E("RegenerateAssistantResponse: сброс черновика: %v", err)
		return nil, err
	}

	messageID := assistantMessageID

	responseChan, err := c.llmRepo.SendMessageOnRunner(ctx, runnerAddr, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
	if err != nil {
		logger.E("RegenerateAssistantResponse: LLM: %v", err)
		return nil, err
	}

	var fullResponse strings.Builder
	clientChan := make(chan ChatStreamChunk, 100)
	go func() {
		defer func() {
			newContent := fullResponse.String()
			_ = c.messageRepo.UpdateContent(context.Background(), messageID, newContent)
			if c.assistantRegenRepo != nil && strings.TrimSpace(oldContent) != "" && strings.TrimSpace(newContent) != "" {
				_ = c.assistantRegenRepo.Create(context.Background(), &domain.AssistantMessageRegeneration{
					SessionId:   sessionId,
					MessageId:   messageID,
					RegenUserId: userId,
					OldContent:  oldContent,
					NewContent:  newContent,
					CreatedAt:   time.Now(),
				})
			}
		}()
		defer close(clientChan)

		if regenHistoryNotice {
			select {
			case <-ctx.Done():
				return
			case clientChan <- ChatStreamChunk{Kind: StreamChunkKindNotice, Text: HistoryTruncatedClientNotice}:
			}
		}

		forwardLLMStreamChunks(ctx, clientChan, messageID, responseChan, &fullResponse)
	}()

	return clientChan, nil
}

func (c *ChatUseCase) ContinueAssistantResponse(ctx context.Context, userId int, sessionId int64, assistantMessageID int64) (chan ChatStreamChunk, error) {
	logger.D("ContinueAssistantResponse: session=%d user=%d assistantMsg=%d", sessionId, userId, assistantMessageID)
	if assistantMessageID <= 0 {
		return nil, fmt.Errorf("некорректный assistant_message_id")
	}

	session, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		logger.W("ContinueAssistantResponse: сессия: %v", err)
		return nil, err
	}
	runnerAddr, resolvedModel, err := c.chatRunnerAddrAndModel(ctx, session)
	if err != nil {
		return nil, err
	}

	target, err := c.messageRepo.GetByID(ctx, assistantMessageID)
	if err != nil {
		logger.E("ContinueAssistantResponse: загрузка сообщения: %v", err)
		return nil, err
	}
	if target == nil || target.SessionId != sessionId {
		return nil, fmt.Errorf("сообщение не найдено")
	}
	if target.Role != domain.MessageRoleAssistant {
		return nil, fmt.Errorf("продолжить можно только ответ ассистента")
	}
	existingContent := target.Content
	if strings.TrimSpace(existingContent) == "" {
		return nil, fmt.Errorf("нет частичного ответа для продолжения")
	}

	maxID, err := c.messageRepo.MaxMessageIDInSession(ctx, sessionId)
	if err != nil {
		logger.E("ContinueAssistantResponse: max id: %v", err)
		return nil, err
	}
	if maxID != assistantMessageID {
		return nil, fmt.Errorf("продолжить можно только последнее сообщение в чате")
	}

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	if len(parseToolsJSON(settings.ToolsJSON)) > 0 {
		return nil, domain.ErrRegenerateToolsNotSupported
	}
	if settings.MCPEnabled && len(settings.MCPServerIDs) > 0 {
		return nil, domain.ErrRegenerateToolsNotSupported
	}
	stopSequences, timeoutSeconds, genParams := genParamsFromSessionSettings(settings)

	rawPrefix, err := c.messageRepo.ListMessagesWithIDLessThan(ctx, sessionId, assistantMessageID)
	if err != nil {
		logger.E("ContinueAssistantResponse: префикс истории: %v", err)
		return nil, err
	}
	messages := filterHistoryForLLM(rawPrefix)

	partialForLLM := *target
	partialForLLM.Content = existingContent
	userContinue := domain.NewMessage(sessionId, "Продолжите ваш предыдущий ответ в роли ассистента. Выведите только продолжение текста, не повторяя то, что уже написали выше.", domain.MessageRoleUser)

	messagesForLLM := make([]*domain.Message, 0, len(messages)+3)
	messagesForLLM = append(messagesForLLM, chatSessionSystemMessage(sessionId, settings))
	messagesForLLM = append(messagesForLLM, messages...)
	messagesForLLM = append(messagesForLLM, &partialForLLM, userContinue)

	if err := c.hydrateAttachmentsForRunner(ctx, messagesForLLM); err != nil {
		logger.E("ContinueAssistantResponse: вложения: %v", err)
		return nil, err
	}
	var contHistoryNotice bool
	messagesForLLM, contHistoryNotice = c.capLLMHistoryTokens(ctx, messagesForLLM, 2, sessionId, resolvedModel, runnerAddr, true)

	messageID := assistantMessageID

	responseChan, err := c.llmRepo.SendMessageOnRunner(ctx, runnerAddr, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
	if err != nil {
		logger.E("ContinueAssistantResponse: LLM: %v", err)
		return nil, err
	}

	var newPart strings.Builder
	clientChan := make(chan ChatStreamChunk, 100)
	go func() {
		defer func() {
			_ = c.messageRepo.UpdateContent(context.Background(), messageID, existingContent+newPart.String())
		}()
		defer close(clientChan)

		if contHistoryNotice {
			select {
			case <-ctx.Done():
				return
			case clientChan <- ChatStreamChunk{Kind: StreamChunkKindNotice, Text: HistoryTruncatedClientNotice}:
			}
		}

		forwardLLMStreamChunks(ctx, clientChan, messageID, responseChan, &newPart)
	}()

	return clientChan, nil
}

func (c *ChatUseCase) EditUserMessageAndContinue(ctx context.Context, userId int, sessionId int64, userMessageID int64, newContent string) (chan ChatStreamChunk, error) {
	logger.D("EditUserMessageAndContinue: session=%d user=%d userMsg=%d", sessionId, userId, userMessageID)
	if userMessageID <= 0 {
		return nil, fmt.Errorf("некорректный user_message_id")
	}
	newContent = strings.TrimSpace(newContent)
	if newContent == "" {
		return nil, fmt.Errorf("new_content не может быть пустым")
	}

	session, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		return nil, err
	}
	runnerAddr, resolvedModel, err := c.chatRunnerAddrAndModel(ctx, session)
	if err != nil {
		return nil, err
	}

	target, err := c.messageRepo.GetByID(ctx, userMessageID)
	if err != nil {
		return nil, err
	}
	if target == nil || target.SessionId != sessionId {
		return nil, fmt.Errorf("сообщение не найдено")
	}
	if target.Role != domain.MessageRoleUser {
		return nil, fmt.Errorf("редактировать можно только user-сообщение")
	}

	maxID, err := c.messageRepo.MaxMessageIDInSession(ctx, sessionId)
	if err != nil {
		return nil, err
	}

	oldContent := target.Content
	edit := &domain.MessageEdit{
		SessionId:       sessionId,
		MessageId:       userMessageID,
		EditorUserId:    userId,
		OldContent:      oldContent,
		NewContent:      newContent,
		SoftDeletedFrom: userMessageID,
		SoftDeletedTo:   maxID,
		CreatedAt:       time.Now(),
	}
	if err := c.chatTx.WithinTx(ctx, func(ctx context.Context, r domain.ChatRepos) error {
		if err := r.Message.UpdateContent(ctx, userMessageID, newContent); err != nil {
			return err
		}
		if c.messageEditRepo != nil {
			if err := r.MessageEdit.Create(ctx, edit); err != nil {
				return err
			}
		}
		return r.Message.SoftDeleteRangeAfterID(ctx, sessionId, userMessageID, maxID)
	}); err != nil {
		return nil, err
	}

	rawPrefix, err := c.messageRepo.ListMessagesUpToID(ctx, sessionId, userMessageID)
	if err != nil {
		return nil, err
	}
	messages := filterHistoryForLLM(rawPrefix)

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	stopSequences, timeoutSeconds, genParams := genParamsFromSessionSettings(settings)
	c.maybeInjectWebSearchTool(ctx, genParams, settings)
	c.maybeInjectMCPTools(ctx, genParams, settings, userId)
	c.maybeInjectMCPBuiltinMetaTools(genParams, settings)

	messagesForLLM := make([]*domain.Message, 0, len(messages)+1)
	messagesForLLM = append(messagesForLLM, c.llmChatSystemMessage(ctx, sessionId, settings, userId))
	messagesForLLM = append(messagesForLLM, messages...)

	if err := c.hydrateAttachmentsForRunner(ctx, messagesForLLM); err != nil {
		return nil, err
	}
	var editHistoryNotice bool
	messagesForLLM, editHistoryNotice = c.capLLMHistoryTokens(ctx, messagesForLLM, 1, sessionId, resolvedModel, runnerAddr, true)

	if genParams != nil && len(genParams.Tools) > 0 {
		return c.sendMessageWithToolLoop(ctx, userId, sessionId, runnerAddr, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams, editHistoryNotice)
	}

	assistantMsg := domain.NewMessage(sessionId, "", domain.MessageRoleAssistant)
	if err := c.messageRepo.Create(ctx, assistantMsg); err != nil {
		return nil, err
	}
	messageID := assistantMsg.Id

	responseChan, err := c.llmRepo.SendMessageOnRunner(ctx, runnerAddr, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
	if err != nil {
		return nil, err
	}

	var fullResponse strings.Builder
	clientChan := make(chan ChatStreamChunk, 100)
	go func() {
		defer func() {
			_ = c.messageRepo.UpdateContent(context.Background(), messageID, fullResponse.String())
		}()
		defer close(clientChan)

		if editHistoryNotice {
			select {
			case <-ctx.Done():
				return
			case clientChan <- ChatStreamChunk{Kind: StreamChunkKindNotice, Text: HistoryTruncatedClientNotice}:
			}
		}

		forwardLLMStreamChunks(ctx, clientChan, messageID, responseChan, &fullResponse)
	}()

	return clientChan, nil
}

func (c *ChatUseCase) GetUserMessageEdits(ctx context.Context, userId int, sessionId int64, userMessageID int64) ([]*domain.MessageEdit, error) {
	if userMessageID <= 0 {
		return nil, fmt.Errorf("некорректный user_message_id")
	}
	if _, err := c.verifySessionOwnership(ctx, userId, sessionId); err != nil {
		return nil, err
	}
	target, err := c.messageRepo.GetByID(ctx, userMessageID)
	if err != nil {
		return nil, err
	}
	if target == nil || target.SessionId != sessionId || target.Role != domain.MessageRoleUser {
		return nil, fmt.Errorf("сообщение не найдено")
	}
	if c.messageEditRepo == nil {
		return []*domain.MessageEdit{}, nil
	}
	return c.messageEditRepo.ListByMessageID(ctx, userMessageID, 50)
}

func (c *ChatUseCase) GetAssistantMessageRegenerations(ctx context.Context, userId int, sessionId int64, assistantMessageID int64) ([]*domain.AssistantMessageRegeneration, error) {
	if assistantMessageID <= 0 {
		return nil, fmt.Errorf("некорректный assistant_message_id")
	}

	if _, err := c.verifySessionOwnership(ctx, userId, sessionId); err != nil {
		return nil, err
	}

	target, err := c.messageRepo.GetByID(ctx, assistantMessageID)
	if err != nil {
		return nil, err
	}

	if target == nil || target.SessionId != sessionId || target.Role != domain.MessageRoleAssistant {
		return nil, fmt.Errorf("сообщение не найдено")
	}

	if c.assistantRegenRepo == nil {
		return []*domain.AssistantMessageRegeneration{}, nil
	}

	return c.assistantRegenRepo.ListByMessageID(ctx, assistantMessageID, 50)
}

func (c *ChatUseCase) GetSessionMessagesForAssistantMessageVersion(ctx context.Context, userId int, sessionId int64, assistantMessageID int64, versionIndex int32) ([]*domain.Message, error) {
	if assistantMessageID <= 0 {
		return nil, fmt.Errorf("некорректный assistant_message_id")
	}
	if versionIndex < 0 {
		return nil, fmt.Errorf("некорректный version_index")
	}
	if _, err := c.verifySessionOwnership(ctx, userId, sessionId); err != nil {
		return nil, err
	}
	target, err := c.messageRepo.GetByID(ctx, assistantMessageID)
	if err != nil {
		return nil, err
	}
	if target == nil || target.SessionId != sessionId || target.Role != domain.MessageRoleAssistant {
		return nil, fmt.Errorf("сообщение не найдено")
	}

	prefix, err := c.messageRepo.ListMessagesUpToID(ctx, sessionId, assistantMessageID)
	if err != nil {
		return nil, err
	}

	if c.assistantRegenRepo == nil {
		return prefix, nil
	}

	desc, err := c.assistantRegenRepo.ListByMessageID(ctx, assistantMessageID, 200)
	if err != nil {
		return nil, err
	}

	regens := make([]*domain.AssistantMessageRegeneration, 0, len(desc))
	for i := len(desc) - 1; i >= 0; i-- {
		regens = append(regens, desc[i])
	}

	n := int32(len(regens))
	if versionIndex > n {
		versionIndex = n
	}

	for i := range prefix {
		if prefix[i] == nil || prefix[i].Id != assistantMessageID {
			continue
		}

		if len(regens) == 0 {
			break
		}

		if versionIndex == 0 {
			prefix[i].Content = regens[0].OldContent
		} else {
			prefix[i].Content = regens[versionIndex-1].NewContent
		}
		break
	}

	return prefix, nil
}

func (c *ChatUseCase) GetSessionMessagesForUserMessageVersion(ctx context.Context, userId int, sessionId int64, userMessageID int64, versionIndex int32) ([]*domain.Message, error) {
	if userMessageID <= 0 {
		return nil, fmt.Errorf("некорректный user_message_id")
	}

	if versionIndex < 0 {
		return nil, fmt.Errorf("некорректный version_index")
	}

	if _, err := c.verifySessionOwnership(ctx, userId, sessionId); err != nil {
		return nil, err
	}

	target, err := c.messageRepo.GetByID(ctx, userMessageID)
	if err != nil {
		return nil, err
	}

	if target == nil || target.SessionId != sessionId || target.Role != domain.MessageRoleUser {
		return nil, fmt.Errorf("сообщение не найдено")
	}

	if c.messageEditRepo == nil {
		raw, _, err := c.messageRepo.GetBySessionId(ctx, sessionId, 1, 200)
		return raw, err
	}

	editsDesc, err := c.messageEditRepo.ListByMessageID(ctx, userMessageID, 200)
	if err != nil {
		return nil, err
	}

	edits := make([]*domain.MessageEdit, 0, len(editsDesc))
	for i := len(editsDesc) - 1; i >= 0; i-- {
		edits = append(edits, editsDesc[i])
	}
	n := int32(len(edits))
	if versionIndex > n {
		versionIndex = n
	}

	prefix, err := c.messageRepo.ListMessagesUpToID(ctx, sessionId, userMessageID)
	if err != nil {
		return nil, err
	}

	if len(prefix) > 0 {
		for i := range prefix {
			if prefix[i] != nil && prefix[i].Id == userMessageID {
				if len(edits) > 0 {
					if versionIndex == 0 {
						prefix[i].Content = edits[0].OldContent
					} else {
						prefix[i].Content = edits[versionIndex-1].NewContent
					}
				}
				break
			}
		}
	}

	var from time.Time
	var to time.Time
	if len(edits) == 0 {
		return prefix, nil
	}
	if versionIndex == 0 {
		from = target.CreatedAt
		to = edits[0].CreatedAt
	} else {
		from = edits[versionIndex-1].CreatedAt
		if versionIndex < int32(len(edits)) {
			to = edits[versionIndex].CreatedAt
		} else {
			to = time.Now().Add(365 * 24 * time.Hour)
		}
	}

	windowMsgs, err := c.messageRepo.ListBySessionCreatedAtWindowIncludingDeleted(ctx, sessionId, from, to)
	if err != nil {
		return nil, err
	}

	tail := make([]*domain.Message, 0, len(windowMsgs))
	for _, m := range windowMsgs {
		if m == nil {
			continue
		}

		if m.Id <= userMessageID {
			continue
		}

		tail = append(tail, m)
	}

	out := append([]*domain.Message{}, prefix...)
	out = append(out, tail...)

	return out, nil
}

func (c *ChatUseCase) GetSessionSettings(ctx context.Context, userId int, sessionID int64) (*domain.ChatSessionSettings, error) {
	_, err := c.verifySessionOwnership(ctx, userId, sessionID)
	if err != nil {
		return nil, err
	}

	return c.sessionSettingsRepo.GetBySessionID(ctx, sessionID)
}

func (c *ChatUseCase) UpdateSessionSettings(
	ctx context.Context,
	userId int,
	sessionID int64,
	systemPrompt string,
	stopSequences []string,
	timeoutSeconds int32,
	temperature *float32,
	topK *int32,
	topP *float32,
	jsonMode bool,
	jsonSchema string,
	toolsJSON string,
	profile string,
	modelReasoningEnabled bool,
	webSearchEnabled bool,
	webSearchProvider string,
	mcpEnabled bool,
	mcpServerIDs []int64,
) (*domain.ChatSessionSettings, error) {
	_, err := c.verifySessionOwnership(ctx, userId, sessionID)
	if err != nil {
		return nil, err
	}
	if stopSequences == nil {
		stopSequences = []string{}
	}
	if mcpServerIDs == nil {
		mcpServerIDs = []int64{}
	}
	for _, mid := range mcpServerIDs {
		if mid <= 0 {
			continue
		}
		if _, err := c.mcpServerRepo.GetByIDAccessible(ctx, mid, userId); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil, fmt.Errorf("недопустимый MCP-сервер id=%d", mid)
			}
			return nil, err
		}
	}
	settings := &domain.ChatSessionSettings{
		SessionID:             sessionID,
		SystemPrompt:          strings.TrimSpace(systemPrompt),
		StopSequences:         stopSequences,
		TimeoutSeconds:        timeoutSeconds,
		Temperature:           temperature,
		TopK:                  topK,
		TopP:                  topP,
		JSONMode:              jsonMode,
		JSONSchema:            strings.TrimSpace(jsonSchema),
		ToolsJSON:             strings.TrimSpace(toolsJSON),
		Profile:               strings.TrimSpace(profile),
		ModelReasoningEnabled: modelReasoningEnabled,
		WebSearchEnabled:      webSearchEnabled,
		WebSearchProvider:     normalizeWebSearchProvider(webSearchProvider),
		MCPEnabled:            mcpEnabled,
		MCPServerIDs:          mcpServerIDs,
	}
	if err := c.sessionSettingsRepo.Upsert(ctx, settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func parseToolsJSON(raw string) []domain.Tool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var tools []domain.Tool
	if err := json.Unmarshal([]byte(trimmed), &tools); err != nil {
		return nil
	}
	return tools
}

func (c *ChatUseCase) CreateSession(ctx context.Context, userId int, title string) (*domain.ChatSession, error) {
	if strings.TrimSpace(title) == "" {
		title = "Чат от " + time.Now().Format("15:04:05 02.01.2006")
	}

	first, err := c.runnerRepo.FirstEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if first == nil {
		return nil, domain.ErrNoRunners
	}
	if strings.TrimSpace(first.SelectedModel) == "" {
		return nil, domain.ErrRunnerChatModelNotConfigured
	}

	rid := first.ID
	session := domain.NewChatSession(userId, title)
	session.SelectedRunnerID = &rid
	if err := c.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

func (c *ChatUseCase) GetSession(ctx context.Context, userId int, sessionID int64) (*domain.ChatSession, error) {
	return c.verifySessionOwnership(ctx, userId, sessionID)
}

func (c *ChatUseCase) GetSessions(ctx context.Context, userId int, page, pageSize int32) ([]*domain.ChatSession, int32, error) {
	return c.sessionRepo.GetByUserId(ctx, userId, page, pageSize)
}

func (c *ChatUseCase) GetSessionMessages(ctx context.Context, userId int, sessionId int64, beforeMessageID int64, pageSize int32) ([]*domain.Message, int32, bool, error) {
	_, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		return nil, 0, false, err
	}

	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	msgs, total, err := c.messageRepo.ListBySessionBeforeID(ctx, sessionId, beforeMessageID, pageSize)
	if err != nil {
		return nil, 0, false, err
	}
	hasMoreOlder := false
	if len(msgs) > 0 {
		if int32(len(msgs)) < pageSize {
			hasMoreOlder = false
		} else {
			hasMoreOlder, err = c.messageRepo.SessionHasOlderMessages(ctx, sessionId, msgs[0].Id)
			if err != nil {
				return nil, 0, false, err
			}
		}
	}

	return msgs, total, hasMoreOlder, nil
}

func (c *ChatUseCase) DeleteSession(ctx context.Context, userId int, sessionID int64) error {
	_, err := c.verifySessionOwnership(ctx, userId, sessionID)
	if err != nil {
		return err
	}

	return c.sessionRepo.Delete(ctx, sessionID)
}

func (c *ChatUseCase) UpdateSessionTitle(ctx context.Context, userId int, sessionId int64, title string) (*domain.ChatSession, error) {
	session, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		return nil, err
	}

	session.Title = title
	if err := c.sessionRepo.Update(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

func (c *ChatUseCase) hydrateAttachmentsForRunner(ctx context.Context, messages []*domain.Message) error {
	if len(messages) == 0 {
		return nil
	}

	if strings.TrimSpace(c.attachmentsSaveDir) == "" {
		for _, m := range messages {
			if m != nil && m.AttachmentFileID != nil && len(m.AttachmentContent) == 0 {
				return fmt.Errorf("вложение в истории чата (file_id=%d): не задан каталог вложений", *m.AttachmentFileID)
			}
		}
		return nil
	}

	needIDs := make(map[int64]struct{})
	for _, m := range messages {
		if m == nil || m.AttachmentFileID == nil || len(m.AttachmentContent) > 0 {
			continue
		}
		needIDs[*m.AttachmentFileID] = struct{}{}
	}

	if len(needIDs) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(needIDs))
	for id := range needIDs {
		ids = append(ids, id)
	}

	files, err := c.fileRepo.ListByIds(ctx, ids)
	if err != nil {
		return fmt.Errorf("пакетная загрузка файлов вложений: %w", err)
	}

	byID := make(map[int64]*domain.File, len(files))
	for _, f := range files {
		if f != nil {
			byID[f.Id] = f
		}
	}

	for id := range needIDs {
		if _, ok := byID[id]; !ok {
			return fmt.Errorf("файл вложения id=%d не найден", id)
		}
	}

	var toHydrate []*domain.Message
	for _, m := range messages {
		if m == nil || m.AttachmentFileID == nil || len(m.AttachmentContent) > 0 {
			continue
		}
		toHydrate = append(toHydrate, m)
	}
	if len(toHydrate) == 0 {
		return nil
	}

	sem := make(chan struct{}, normalizeAttachmentHydrateParallelism(c.attachmentHydrateParallelism))
	g, gctx := errgroup.WithContext(ctx)
	for _, m := range toHydrate {
		f := byID[*m.AttachmentFileID]
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			return c.hydrateOneAttachmentForRunner(gctx, m, f)
		})
	}
	return g.Wait()
}

func (c *ChatUseCase) hydrateOneAttachmentForRunner(ctx context.Context, m *domain.Message, f *domain.File) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if f.ExpiresAt != nil && time.Now().After(*f.ExpiresAt) {
		return fmt.Errorf("файл вложения id=%d: истёк срок хранения", *m.AttachmentFileID)
	}

	path := strings.TrimSpace(f.StoragePath)
	if path == "" {
		return fmt.Errorf("файл вложения id=%d: пустой storage_path", *m.AttachmentFileID)
	}

	expectedDir := filepath.Clean(filepath.Join(c.attachmentsSaveDir, strconv.FormatInt(m.SessionId, 10)))
	cleanPath := filepath.Clean(path)
	if !strings.HasPrefix(cleanPath, expectedDir+string(filepath.Separator)) && cleanPath != expectedDir {
		return fmt.Errorf("файл вложения id=%d: путь вне каталога сессии", *m.AttachmentFileID)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("чтение вложения %q: %w", path, err)
	}

	if len(data) > document.MaxRecommendedAttachmentSizeBytes {
		return fmt.Errorf("вложение %q превышает лимит %d байт", path, document.MaxRecommendedAttachmentSizeBytes)
	}

	name := strings.TrimSpace(m.AttachmentName)
	if name == "" {
		name = filepath.Base(f.Filename)
	}

	if document.IsImageAttachment(name) {
		if err := document.ValidateImageAttachment(name, data); err != nil {
			return err
		}
		m.AttachmentContent = data
		return nil
	}

	if err := document.ValidateAttachment(name, data); err != nil {
		return err
	}

	sum := sha256.Sum256(data)
	shaHex := hex.EncodeToString(sum[:])

	var built string
	if strings.EqualFold(f.ExtractedTextContentSha256, shaHex) && strings.TrimSpace(f.ExtractedText) != "" {
		var err error
		built, err = buildExpandedAttachmentMessage(name, f.ExtractedText, m.Content)
		if err != nil {
			return err
		}
	} else {
		extracted, err := document.ExtractText(name, data)
		if err != nil {
			logger.W("ChatUseCase: извлечение текста из вложения %q: %v", name, err)
			return fmt.Errorf("%w: %v", document.ErrTextExtractionFailed, err)
		}

		built, err = buildExpandedAttachmentMessage(name, extracted, m.Content)
		if err != nil {
			return err
		}

		if len(extracted) <= maxFileExtractedTextCacheBytes {
			if err := c.fileRepo.SaveExtractedTextCache(ctx, f.Id, shaHex, extracted); err != nil {
				logger.W("ChatUseCase: кэш извлечённого текста file_id=%d: %v", f.Id, err)
			}
		}
	}

	m.Content = built
	return nil
}

func (c *ChatUseCase) loadSessionAttachmentForSend(ctx context.Context, userID int, sessionID int64, fileID int64) (name string, content []byte, err error) {
	if strings.TrimSpace(c.attachmentsSaveDir) == "" {
		return "", nil, fmt.Errorf("хранилище вложений не настроено")
	}

	f, err := c.fileRepo.GetById(ctx, fileID)
	if err != nil {
		return "", nil, fmt.Errorf("файл id=%d: %w", fileID, err)
	}

	if f == nil {
		return "", nil, fmt.Errorf("файл id=%d не найден", fileID)
	}

	if f.ChatSessionID == nil || *f.ChatSessionID != sessionID {
		return "", nil, fmt.Errorf("файл не относится к этой сессии")
	}

	if f.UserID == nil || *f.UserID != userID {
		return "", nil, fmt.Errorf("файл не принадлежит пользователю")
	}

	if f.ExpiresAt != nil && time.Now().After(*f.ExpiresAt) {
		return "", nil, fmt.Errorf("срок действия файла истёк")
	}

	path := strings.TrimSpace(f.StoragePath)
	if path == "" {
		return "", nil, fmt.Errorf("файл id=%d: пустой storage_path", fileID)
	}

	expectedDir := filepath.Clean(filepath.Join(c.attachmentsSaveDir, strconv.FormatInt(sessionID, 10)))
	cleanPath := filepath.Clean(path)
	if !strings.HasPrefix(cleanPath, expectedDir+string(filepath.Separator)) && cleanPath != expectedDir {
		return "", nil, fmt.Errorf("файл id=%d: неверный путь хранения", fileID)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("чтение файла: %w", err)
	}

	if len(data) > document.MaxRecommendedAttachmentSizeBytes {
		return "", nil, fmt.Errorf("размер вложения превышает рекомендуемый максимум: %d байт", document.MaxRecommendedAttachmentSizeBytes)
	}

	baseName := filepath.Base(f.Filename)
	if baseName == "" || baseName == "." {
		baseName = "file"
	}

	if document.IsImageAttachment(baseName) {
		if err := document.ValidateImageAttachment(baseName, data); err != nil {
			return "", nil, err
		}
	} else if err := document.ValidateAttachment(baseName, data); err != nil {
		return "", nil, err
	}

	return baseName, data, nil
}

func (c *ChatUseCase) historyMessagesForLLM(ctx context.Context, sessionId int64) ([]*domain.Message, error) {
	limit := int32(0)
	if c.runnerReg != nil {
		limit = int32(c.runnerReg.AggregateChatHints().LLMHistoryMaxMessages)
	}
	raw, err := c.messageRepo.ListLatestMessagesForSession(ctx, sessionId, limit)
	if err != nil {
		return nil, err
	}
	return filterHistoryForLLM(raw), nil
}

func (c *ChatUseCase) aggregateMaxContextTokens() int {
	maxTok := 0
	if c.runnerReg != nil {
		maxTok = c.runnerReg.AggregateChatHints().MaxContextTokens
	}

	if maxTok <= 0 && c.llmContextFallbackTokens > 0 {
		maxTok = normalizeLLMHistoryApproxMaxTokens(c.llmContextFallbackTokens)
	}

	return maxTok
}

func (c *ChatUseCase) effectiveMaxRAGContextRunes(systemAndHistory []*domain.Message, userMessage string) int {
	cap := maxFileRAGContextRunesCeiling
	maxTok := c.aggregateMaxContextTokens()
	if maxTok <= 0 {
		const blindRAGRunes = 3200
		if blindRAGRunes < cap {
			return blindRAGRunes
		}

		return cap
	}

	pre := sumApproxTokens(systemAndHistory)
	const genReserve = 512
	userOverhead := utf8.RuneCountInString(strings.TrimSpace(userMessage)) / 2
	if userOverhead < 32 {
		userOverhead = 32
	}

	ragTok := maxTok - pre - genReserve - userOverhead
	if ragTok < 120 {
		ragTok = 120
	}

	runesLimit := ragTok * 2
	if runesLimit > cap {
		runesLimit = cap
	}

	if runesLimit < 200 {
		runesLimit = 200
	}

	return runesLimit
}

func (c *ChatUseCase) capLLMHistoryTokens(ctx context.Context, msgs []*domain.Message, tailPreserve int, sessionID int64, resolvedModel string, chatRunnerAddr string, allowSummarize bool) ([]*domain.Message, bool) {
	maxTok := c.aggregateMaxContextTokens()
	if maxTok <= 0 {
		return msgs, false
	}

	out, trimmed, dropped := trimLLMMessagesByApproxTokensWithDropped(msgs, maxTok, tailPreserve)
	if !trimmed {
		return out, false
	}

	summarizeDropped := false
	if c.runnerReg != nil {
		summarizeDropped = c.runnerReg.AggregateChatHints().LLMHistorySummarizeDropped
	}

	logger.I("ChatUseCase: session=%d промпт усечён по оценке токенов (~лимит %d): сообщений %d -> %d", sessionID, maxTok, len(msgs), len(out))
	if allowSummarize && summarizeDropped && len(dropped) > 0 {
		if sum := strings.TrimSpace(c.summarizeDroppedMessages(ctx, sessionID, resolvedModel, chatRunnerAddr, dropped)); sum != "" {
			out = injectSummaryAfterSystem(out, sum)
			out2, trimmedAgain, _ := trimLLMMessagesByApproxTokensWithDropped(out, maxTok, tailPreserve)
			if trimmedAgain {
				logger.W("ChatUseCase: session=%d после вставки резюме снова усечено: %d -> %d сообщений",
					sessionID, len(out), len(out2))
			}
			out = out2
		}
	}

	return out, true
}

func filterHistoryForLLM(messages []*domain.Message) []*domain.Message {
	if len(messages) == 0 {
		return nil
	}

	out := make([]*domain.Message, 0, len(messages))
	for _, m := range messages {
		if m == nil {
			continue
		}
		if m.Role == domain.MessageRoleAssistant && strings.TrimSpace(m.Content) == "" {
			if strings.TrimSpace(m.ToolCallsJSON) == "" {
				continue
			}
		}
		out = append(out, m)
	}

	return out
}

const documentAttachmentInstruction = "Ниже - текст вложенного документа. Отвечай, опираясь на него; при необходимости приводи короткие цитаты."
const documentTruncatedNotice = "Внимание: из-за ограничения длины контекста показана только начальная часть файла."

func buildMessageWithFile(attachmentName string, attachmentContent []byte, userMessage string) (string, error) {
	extracted, err := document.ExtractText(attachmentName, attachmentContent)
	if err != nil {
		logger.W("ChatUseCase: извлечение текста из вложения %q: %v", attachmentName, err)
		return "", fmt.Errorf("%w: %v", document.ErrTextExtractionFailed, err)
	}

	return buildExpandedAttachmentMessage(attachmentName, extracted, userMessage)
}

func buildMessageWithRAG(fileName string, userMessage string, scored []domain.ScoredDocumentRAGChunk, maxContextRunes int) string {
	if maxContextRunes < 200 {
		maxContextRunes = 200
	}

	var b strings.Builder
	b.WriteString("Ниже — наиболее релевантные фрагменты из документа (векторный поиск по запросу). Опирайся на них при ответе; при цитировании указывай номер фрагмента.\n\n")
	total := 0
	for i, sc := range scored {
		header := fmt.Sprintf("--- Фрагмент %d (близость=%.3f, chunk_index=%d) ---\n", i+1, sc.Score, sc.DocumentRAGChunk.ChunkIndex)
		body := sc.DocumentRAGChunk.Text
		piece := header + body + "\n\n"
		r := utf8.RuneCountInString(piece)
		if total+r > maxContextRunes {
			room := maxContextRunes - total - utf8.RuneCountInString(header)
			if room < 64 {
				break
			}

			br := []rune(body)
			if len(br) > room {
				body = string(br[:room]) + "\n...(обрезано по лимиту контекста)"
			}
			piece = header + body + "\n\n"
		}

		b.WriteString(piece)
		total += utf8.RuneCountInString(piece)
		if total >= maxContextRunes {
			break
		}
	}

	b.WriteString("Файл: «")
	b.WriteString(fileName)
	b.WriteString("»\n\n---\n\n")
	b.WriteString(strings.TrimSpace(userMessage))

	return b.String()
}

func buildExpandedAttachmentMessage(attachmentName, extractedText, userMessage string) (string, error) {
	fileContent, truncated := document.TruncateExtractedText(extractedText, document.MaxEmbeddedAttachmentTextRunes)

	var b strings.Builder
	b.WriteString(documentAttachmentInstruction)
	b.WriteString("\n\n")
	if truncated {
		b.WriteString(documentTruncatedNotice)
		b.WriteString("\n\n")
	}

	b.WriteString(fmt.Sprintf("Файл «%s»:\n\n```\n%s\n```", attachmentName, fileContent))
	if userMessage != "" {
		b.WriteString("\n\n---\n\n")
		b.WriteString(userMessage)
	}

	return b.String(), nil
}

func (c *ChatUseCase) PutSessionFile(ctx context.Context, userID int, sessionID int64, filename string, content []byte, ttlSeconds int32) (int64, error) {
	if strings.TrimSpace(c.attachmentsSaveDir) == "" {
		return 0, fmt.Errorf("хранилище вложений не настроено")
	}

	if len(content) == 0 {
		return 0, fmt.Errorf("пустой файл")
	}

	if len(content) > document.MaxRecommendedAttachmentSizeBytes {
		return 0, fmt.Errorf("размер файла превышает рекомендуемый максимум: %d байт", document.MaxRecommendedAttachmentSizeBytes)
	}

	filename = strings.TrimSpace(filename)
	baseName := filepath.Base(filename)
	if baseName == "" || baseName == "." {
		return 0, fmt.Errorf("некорректное имя файла")
	}

	if document.IsImageAttachment(baseName) {
		if err := document.ValidateImageAttachment(baseName, content); err != nil {
			return 0, err
		}
	} else if err := document.ValidateAttachment(baseName, content); err != nil {
		return 0, err
	}

	if _, err := c.verifySessionOwnership(ctx, userID, sessionID); err != nil {
		return 0, err
	}

	if !document.IsImageAttachment(baseName) && document.IsSupportedExtension(baseName) {
		extracted, err := document.ExtractText(baseName, content)
		if err != nil {
			return 0, fmt.Errorf("проверка документа при загрузке: %w", err)
		}

		if strings.TrimSpace(extracted) == "" {
			return 0, document.ErrNoExtractableText
		}

		if c.ragMaxExtractedRunesOnUpload > 0 {
			n := utf8.RuneCountInString(extracted)
			if n > c.ragMaxExtractedRunesOnUpload {
				return 0, fmt.Errorf("извлечённый текст слишком длинный (%d символов), лимит %d (rag.max_extracted_runes_on_upload)", n, c.ragMaxExtractedRunesOnUpload)
			}
		}
	}

	ttl := int64(ttlSeconds)
	if ttl <= 0 {
		ttl = sessionArtifactDefaultTTL
	}

	if ttl < sessionArtifactMinTTL {
		ttl = sessionArtifactMinTTL
	}

	if ttl > sessionArtifactMaxTTL {
		ttl = sessionArtifactMaxTTL
	}

	n, sum, err := c.fileRepo.CountSessionTTLArtifacts(ctx, sessionID, userID)
	if err != nil {
		return 0, err
	}

	if n >= maxSessionArtifactFileCount {
		return 0, fmt.Errorf("слишком много временных файлов в сессии (лимит %d)", maxSessionArtifactFileCount)
	}

	if sum+int64(len(content)) > maxSessionArtifactTotalBytes {
		return 0, fmt.Errorf("превышена квота размера временных файлов сессии")
	}

	exp := time.Now().Add(time.Duration(ttl) * time.Second)
	file, err := c.saveFileInSession(ctx, userID, sessionID, baseName, content, sessionFileKindArtifact, &exp)
	if err != nil {
		return 0, err
	}

	c.scheduleSessionFileRAGIndex(userID, sessionID, file.Id, baseName)
	return file.Id, nil
}

func (c *ChatUseCase) scheduleSessionFileRAGIndex(userID int, sessionID, fileID int64, baseName string) {
	if c.documentIngest == nil || document.IsImageAttachment(baseName) {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), c.ragBackgroundIndexTimeout)
		defer cancel()
		if err := c.documentIngest.IndexSessionFile(ctx, userID, sessionID, fileID, ""); err != nil {
			logger.W("ChatUseCase: фоновая индексация RAG file_id=%d: %v", fileID, err)
		}
	}()
}

func (c *ChatUseCase) GetSessionFile(ctx context.Context, userID int, sessionID int64, fileID int64) (filename string, content []byte, err error) {
	if fileID <= 0 {
		return "", nil, fmt.Errorf("некорректный file_id")
	}

	return c.loadSessionAttachmentForSend(ctx, userID, sessionID, fileID)
}

const (
	sessionFileKindArtifact = "artifact"

	sessionArtifactMinTTL        = int64(60)
	sessionArtifactMaxTTL        = int64(7 * 24 * 3600)
	sessionArtifactDefaultTTL    = int64(24 * 3600)
	maxSessionArtifactFileCount  = 200
	maxSessionArtifactTotalBytes = 80 * 1024 * 1024
)

func (c *ChatUseCase) saveFileInSession(ctx context.Context, userID int, sessionID int64, baseName string, content []byte, kind string, expiresAt *time.Time) (*domain.File, error) {
	dir := filepath.Join(c.attachmentsSaveDir, strconv.FormatInt(sessionID, 10))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	sid := sessionID
	uid := userID
	file := domain.NewFile(baseName, "", int64(len(content)), ".")
	file.ChatSessionID = &sid
	file.UserID = &uid
	file.Kind = kind
	file.ExpiresAt = expiresAt
	if err := c.fileRepo.Create(ctx, file); err != nil {
		return nil, err
	}
	storageName := fmt.Sprintf("%d_%s", file.Id, baseName)
	storagePath := filepath.Join(dir, storageName)
	if err := os.WriteFile(storagePath, content, 0644); err != nil {
		return nil, err
	}
	if err := c.fileRepo.UpdateStoragePath(ctx, file.Id, storagePath); err != nil {
		return nil, err
	}
	file.StoragePath = storagePath
	return file, nil
}

func (c *ChatUseCase) ApplySpreadsheet(
	_ context.Context,
	workbook []byte,
	operationsJSON string,
	previewSheet string,
	previewRange string,
) (workbookOut []byte, previewTSV string, exportedCSV string, err error) {
	return spreadsheet.Apply(workbook, operationsJSON, previewSheet, previewRange)
}

func (c *ChatUseCase) BuildDocx(_ context.Context, specJSON string) ([]byte, error) {
	return document.BuildDOCXFromSpecJSON(specJSON)
}

func (c *ChatUseCase) ApplyMarkdownPatch(_ context.Context, baseText, patchJSON string) (string, error) {
	return document.ApplyMarkdownPatchJSON(baseText, patchJSON)
}
