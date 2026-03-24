package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
)

type ChatUseCase struct {
	sessionRepo         domain.ChatSessionRepository
	preferenceRepo      domain.ChatPreferenceRepository
	sessionSettingsRepo domain.ChatSessionSettingsRepository
	messageRepo         domain.MessageRepository
	fileRepo            domain.FileRepository
	llmRepo             domain.LLMRepository
	attachmentsSaveDir  string
}

func NewChatUseCase(
	sessionRepo domain.ChatSessionRepository,
	preferenceRepo domain.ChatPreferenceRepository,
	sessionSettingsRepo domain.ChatSessionSettingsRepository,
	messageRepo domain.MessageRepository,
	fileRepo domain.FileRepository,
	llmRepo domain.LLMRepository,
	attachmentsSaveDir string,
) *ChatUseCase {
	return &ChatUseCase{
		sessionRepo:         sessionRepo,
		preferenceRepo:      preferenceRepo,
		sessionSettingsRepo: sessionSettingsRepo,
		messageRepo:         messageRepo,
		fileRepo:            fileRepo,
		llmRepo:             llmRepo,
		attachmentsSaveDir:  attachmentsSaveDir,
	}
}

func (c *ChatUseCase) GetSelectedRunner(ctx context.Context, userID int) (string, error) {
	return c.preferenceRepo.GetSelectedRunner(ctx, userID)
}

func (c *ChatUseCase) SetSelectedRunner(ctx context.Context, userID int, runner string) error {
	return c.preferenceRepo.SetSelectedRunner(ctx, userID, runner)
}

func (c *ChatUseCase) GetDefaultRunnerModel(ctx context.Context, userID int, runner string) (string, error) {
	return c.preferenceRepo.GetDefaultRunnerModel(ctx, userID, runner)
}

func (c *ChatUseCase) SetDefaultRunnerModel(ctx context.Context, userID int, runner string, model string) error {
	return c.preferenceRepo.SetDefaultRunnerModel(ctx, userID, runner, model)
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

func (c *ChatUseCase) GetModels(ctx context.Context) ([]string, error) {
	return c.llmRepo.GetModels(ctx)
}

func (c *ChatUseCase) Embed(ctx context.Context, userID int, requestedModel string, text string) ([]float32, error) {
	model, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userID, strings.TrimSpace(requestedModel), "")
	if err != nil {
		return nil, err
	}

	return c.llmRepo.Embed(ctx, model, text)
}

func (c *ChatUseCase) EmbedBatch(ctx context.Context, userID int, requestedModel string, texts []string) ([][]float32, error) {
	model, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userID, strings.TrimSpace(requestedModel), "")
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
	genParams = &domain.GenerationParams{
		Temperature: settings.Temperature,
		MaxTokens:   settings.MaxTokens,
		TopK:        settings.TopK,
		TopP:        settings.TopP,
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

func (c *ChatUseCase) SendMessage(ctx context.Context, userId int, sessionId int64, userMessage string, attachmentName string, attachmentContent []byte) (chan string, int64, error) {
	logger.D("SendMessage: session=%d user=%d", sessionId, userId)
	session, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		logger.W("SendMessage: сессия не принадлежит пользователю: %v", err)
		return nil, 0, err
	}

	resolvedModel, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userId, "", session.Model)
	if err != nil {
		logger.W("SendMessage: выбор модели: %v", err)
		return nil, 0, err
	}

	rawMessages, _, err := c.messageRepo.GetBySessionId(ctx, sessionId, 1, 100)
	if err != nil {
		logger.E("SendMessage: получение сообщений: %v", err)
		return nil, 0, err
	}
	messages := filterHistoryForLLM(rawMessages)

	if len(attachmentContent) > 0 || attachmentName != "" {
		if len(attachmentContent) == 0 || strings.TrimSpace(attachmentName) == "" {
			return nil, 0, fmt.Errorf("вложение передано некорректно")
		}

		if len(attachmentContent) > document.MaxRecommendedAttachmentSizeBytes {
			return nil, 0, fmt.Errorf("размер вложения превышает рекомендуемый максимум: %d байт", document.MaxRecommendedAttachmentSizeBytes)
		}

		if err := document.ValidateAttachment(attachmentName, attachmentContent); err != nil {
			return nil, 0, err
		}
	}

	var attachmentFileID *int64
	if len(attachmentContent) > 0 && attachmentName != "" && c.attachmentsSaveDir != "" {
		file, err := c.saveAttachmentAndCreateFile(ctx, sessionId, attachmentName, attachmentContent)
		if err == nil {
			v := file.Id
			attachmentFileID = &v
		} else {
			logger.W("ChatUseCase: вложение: %v", err)
		}
	}

	userMsg := domain.NewMessageWithAttachment(sessionId, userMessage, domain.MessageRoleUser, attachmentFileID)
	if err := c.messageRepo.Create(ctx, userMsg); err != nil {
		logger.E("SendMessage: создание сообщения: %v", err)
		return nil, 0, err
	}

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	messagesForLLM := make([]*domain.Message, 0, len(messages)+2)
	if settings != nil && strings.TrimSpace(settings.SystemPrompt) != "" {
		messagesForLLM = append(messagesForLLM, domain.NewMessage(sessionId, settings.SystemPrompt, domain.MessageRoleSystem))
	}
	messagesForLLM = append(messagesForLLM, messages...)
	if len(attachmentContent) > 0 && attachmentName != "" {
		fullContent := buildMessageWithFile(attachmentName, attachmentContent, userMessage)
		userMsgForLLM := *userMsg
		userMsgForLLM.Content = fullContent
		messagesForLLM = append(messagesForLLM, &userMsgForLLM)
	} else {
		messagesForLLM = append(messagesForLLM, userMsg)
	}

	assistantMsg := domain.NewMessage(sessionId, "", domain.MessageRoleAssistant)
	if err := c.messageRepo.Create(ctx, assistantMsg); err != nil {
		logger.E("SendMessage: создание черновика ответа: %v", err)
		return nil, 0, err
	}
	messageID := assistantMsg.Id

	stopSequences, timeoutSeconds, genParams := genParamsFromSessionSettings(settings)

	responseChan, err := c.llmRepo.SendMessage(ctx, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
	if err != nil {
		logger.E("SendMessage: вызов LLM: %v", err)
		return nil, 0, err
	}
	logger.V("SendMessage: стрим от LLM запущен session=%d", sessionId)

	var fullResponse strings.Builder
	clientChan := make(chan string, 100)
	go func() {
		defer func() {
			_ = c.messageRepo.UpdateContent(context.Background(), messageID, fullResponse.String())
		}()
		defer close(clientChan)

		for chunk := range responseChan {
			fullResponse.WriteString(chunk)
			select {
			case <-ctx.Done():
				return
			case clientChan <- chunk:
			}
		}
	}()

	return clientChan, messageID, nil
}

func (c *ChatUseCase) SendMessageMulti(ctx context.Context, userId int, sessionId int64, turns []*domain.Message) (chan string, int64, error) {
	logger.D("SendMessageMulti: session=%d user=%d turns=%d", sessionId, userId, len(turns))
	session, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		return nil, 0, err
	}

	resolvedModel, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userId, "", session.Model)
	if err != nil {
		return nil, 0, err
	}

	now := time.Now()
	for _, m := range turns {
		if m == nil {
			continue
		}

		row := *m
		row.SessionId = sessionId
		row.Id = 0
		if row.CreatedAt.IsZero() {
			row.CreatedAt = now
			row.UpdatedAt = now
		}

		if err := c.messageRepo.Create(ctx, &row); err != nil {
			logger.E("SendMessageMulti: создание сообщения: %v", err)
			return nil, 0, err
		}
	}

	rawMessages, _, err := c.messageRepo.GetBySessionId(ctx, sessionId, 1, 200)
	if err != nil {
		logger.E("SendMessageMulti: получение сообщений: %v", err)
		return nil, 0, err
	}
	messages := filterHistoryForLLM(rawMessages)

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	messagesForLLM := make([]*domain.Message, 0, len(messages)+1)
	if settings != nil && strings.TrimSpace(settings.SystemPrompt) != "" {
		messagesForLLM = append(messagesForLLM, domain.NewMessage(sessionId, settings.SystemPrompt, domain.MessageRoleSystem))
	}
	messagesForLLM = append(messagesForLLM, messages...)

	assistantMsg := domain.NewMessage(sessionId, "", domain.MessageRoleAssistant)
	if err := c.messageRepo.Create(ctx, assistantMsg); err != nil {
		logger.E("SendMessageMulti: черновик ответа: %v", err)
		return nil, 0, err
	}
	messageID := assistantMsg.Id

	stopSequences, timeoutSeconds, genParams := genParamsFromSessionSettings(settings)
	responseChan, err := c.llmRepo.SendMessage(ctx, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
	if err != nil {
		logger.E("SendMessageMulti: вызов LLM: %v", err)
		return nil, 0, err
	}

	clientChan := make(chan string, 100)
	var fullResponse strings.Builder
	go func() {
		defer func() {
			_ = c.messageRepo.UpdateContent(context.Background(), messageID, fullResponse.String())
		}()
		defer close(clientChan)

		for chunk := range responseChan {
			fullResponse.WriteString(chunk)
			select {
			case <-ctx.Done():
				return
			case clientChan <- chunk:
			}
		}
	}()

	return clientChan, messageID, nil
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
	maxTokens *int32,
	topK *int32,
	topP *float32,
	jsonMode bool,
	jsonSchema string,
	toolsJSON string,
	profile string,
) (*domain.ChatSessionSettings, error) {
	_, err := c.verifySessionOwnership(ctx, userId, sessionID)
	if err != nil {
		return nil, err
	}
	settings := &domain.ChatSessionSettings{
		SessionID:      sessionID,
		SystemPrompt:   strings.TrimSpace(systemPrompt),
		StopSequences:  stopSequences,
		TimeoutSeconds: timeoutSeconds,
		Temperature:    temperature,
		MaxTokens:      maxTokens,
		TopK:           topK,
		TopP:           topP,
		JSONMode:       jsonMode,
		JSONSchema:     strings.TrimSpace(jsonSchema),
		ToolsJSON:      strings.TrimSpace(toolsJSON),
		Profile:        strings.TrimSpace(profile),
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

	resolvedModel, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userId, "", "")
	if err != nil {
		return nil, err
	}
	session := domain.NewChatSession(userId, title, resolvedModel)
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

func (c *ChatUseCase) GetSessionMessages(ctx context.Context, userId int, sessionId int64, page, pageSize int32) ([]*domain.Message, int32, error) {
	_, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		return nil, 0, err
	}

	return c.messageRepo.GetBySessionId(ctx, sessionId, page, pageSize)
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

func buildMessageWithFile(attachmentName string, attachmentContent []byte, userMessage string) string {
	fileContent, err := document.ExtractText(attachmentName, attachmentContent)
	if err != nil {
		logger.W("ChatUseCase: извлечение текста из вложения %q: %v, используем сырое содержимое", attachmentName, err)
		fileContent = string(attachmentContent)
	}
	s := fmt.Sprintf("Файл «%s»:\n\n```\n%s\n```", attachmentName, fileContent)
	if userMessage != "" {
		s += "\n\n---\n\n" + userMessage
	}
	return s
}

func (c *ChatUseCase) saveAttachmentAndCreateFile(ctx context.Context, sessionId int64, attachmentName string, content []byte) (*domain.File, error) {
	baseName := filepath.Base(attachmentName)
	if baseName == "" || baseName == "." {
		baseName = "attachment"
	}
	dir := filepath.Join(c.attachmentsSaveDir, strconv.FormatInt(sessionId, 10))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	file := domain.NewFile(baseName, "", int64(len(content)), ".")
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
