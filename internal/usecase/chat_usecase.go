package usecase

import (
	"context"
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
)

const (
	defaultFileRAGTopK            = 5
	maxFileRAGTopK                = 32
	maxFileRAGContextRunesCeiling = document.MaxEmbeddedAttachmentTextRunes
)

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
	ragNeighborChunkWindow       int
	preferFullDocumentWhenFits   bool
	deepRAGEnabled               bool
	deepRAGMaxMapCalls           int
	deepRAGChunksPerMap          int
	deepRAGMapMaxTokens          int32
	deepRAGMapTimeoutSeconds     int32
	deepRAGMaxMapOutputRunes     int
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
	ragNeighborChunkWindow int,
	preferFullDocumentWhenFits bool,
	deepRAGEnabled bool,
	deepRAGMaxMapCalls int,
	deepRAGChunksPerMap int,
	deepRAGMapMaxTokens int32,
	deepRAGMapTimeoutSeconds int32,
	deepRAGMaxMapOutputRunes int,
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
		ragNeighborChunkWindow:       ragNeighborChunkWindow,
		preferFullDocumentWhenFits:   preferFullDocumentWhenFits,
		deepRAGEnabled:               deepRAGEnabled,
		deepRAGMaxMapCalls:           deepRAGMaxMapCalls,
		deepRAGChunksPerMap:          deepRAGChunksPerMap,
		deepRAGMapMaxTokens:          deepRAGMapMaxTokens,
		deepRAGMapTimeoutSeconds:     deepRAGMapTimeoutSeconds,
		deepRAGMaxMapOutputRunes:     deepRAGMaxMapOutputRunes,
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
