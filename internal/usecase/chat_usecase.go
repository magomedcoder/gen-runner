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
	"github.com/magomedcoder/gen/internal/runner"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
	"github.com/magomedcoder/gen/pkg/spreadsheet"
)

const defaultResponseLanguagePrompt = "Язык ответа: отвечай на том же языке, что и последнее сообщение пользователя в этом запросе. " +
	"Если язык нельзя определить (например, только код, числа или нейтральные символы), отвечай по-русски."

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

type ChatUseCase struct {
	sessionRepo         domain.ChatSessionRepository
	preferenceRepo      domain.ChatPreferenceRepository
	sessionSettingsRepo domain.ChatSessionSettingsRepository
	messageRepo         domain.MessageRepository
	messageEditRepo     domain.MessageEditRepository
	fileRepo            domain.FileRepository
	llmRepo             domain.LLMRepository
	runnerPool          *runner.Pool
	attachmentsSaveDir  string
	defaultRunnerAddr   string
}

func NewChatUseCase(
	sessionRepo domain.ChatSessionRepository,
	preferenceRepo domain.ChatPreferenceRepository,
	sessionSettingsRepo domain.ChatSessionSettingsRepository,
	messageRepo domain.MessageRepository,
	messageEditRepo domain.MessageEditRepository,
	fileRepo domain.FileRepository,
	llmRepo domain.LLMRepository,
	runnerPool *runner.Pool,
	attachmentsSaveDir string,
	defaultRunnerAddr string,
) *ChatUseCase {
	return &ChatUseCase{
		sessionRepo:         sessionRepo,
		preferenceRepo:      preferenceRepo,
		sessionSettingsRepo: sessionSettingsRepo,
		messageRepo:         messageRepo,
		messageEditRepo:     messageEditRepo,
		fileRepo:            fileRepo,
		llmRepo:             llmRepo,
		runnerPool:          runnerPool,
		attachmentsSaveDir:  attachmentsSaveDir,
		defaultRunnerAddr:   strings.TrimSpace(defaultRunnerAddr),
	}
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
	return c.defaultRunnerAddr, nil
}

func (c *ChatUseCase) SetSelectedRunner(ctx context.Context, userID int, runner string) error {
	return c.preferenceRepo.SetSelectedRunner(ctx, userID, runner)
}

func (c *ChatUseCase) GetDefaultRunnerModel(ctx context.Context, userID int, runner string) (string, error) {
	return c.preferenceRepo.GetDefaultRunnerModel(ctx, userID, runner)
}

func (c *ChatUseCase) SetDefaultRunnerModel(ctx context.Context, userID int, runner string, model string) error {
	if err := c.preferenceRepo.SetDefaultRunnerModel(ctx, userID, runner, model); err != nil {
		return err
	}

	if c.runnerPool == nil {
		return nil
	}

	runnerAddr := strings.TrimSpace(runner)
	modelName := strings.TrimSpace(model)
	if runnerAddr == "" {
		return nil
	}

	go func() {
		warmCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		if err := c.runnerPool.WaitRunnerIdle(warmCtx, runnerAddr); err != nil {
			logger.W("SetDefaultRunnerModel: wait idle runner=%s: %v", runnerAddr, err)
			return
		}

		if err := c.runnerPool.UnloadModelOnRunner(warmCtx, runnerAddr); err != nil {
			logger.W("SetDefaultRunnerModel: unload model runner=%s: %v", runnerAddr, err)
		}

		if modelName != "" {
			if err := c.runnerPool.WarmModelOnRunner(warmCtx, runnerAddr, modelName); err != nil {
				logger.W("SetDefaultRunnerModel: warm model runner=%s model=%q: %v", runnerAddr, modelName, err)
			}
		}
	}()

	return nil
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
	model, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userID, strings.TrimSpace(requestedModel), "", c.defaultRunnerAddr)
	if err != nil {
		return nil, err
	}

	return c.llmRepo.Embed(ctx, model, text)
}

func (c *ChatUseCase) EmbedBatch(ctx context.Context, userID int, requestedModel string, texts []string) ([][]float32, error) {
	model, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userID, strings.TrimSpace(requestedModel), "", c.defaultRunnerAddr)
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

func (c *ChatUseCase) SendMessage(ctx context.Context, userId int, sessionId int64, userMessage string, attachmentName string, attachmentContent []byte, existingAttachmentFileID *int64) (chan ChatStreamChunk, error) {
	logger.D("SendMessage: session=%d user=%d", sessionId, userId)
	session, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		logger.W("SendMessage: сессия не принадлежит пользователю: %v", err)
		return nil, err
	}

	resolvedModel, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userId, "", session.Model, c.defaultRunnerAddr)
	if err != nil {
		logger.W("SendMessage: выбор модели: %v", err)
		return nil, err
	}

	rawMessages, _, err := c.messageRepo.GetBySessionId(ctx, sessionId, 1, 100)
	if err != nil {
		logger.E("SendMessage: получение сообщений: %v", err)
		return nil, err
	}
	messages := filterHistoryForLLM(rawMessages)

	var attachmentFileID *int64

	if existingAttachmentFileID != nil && *existingAttachmentFileID > 0 {
		if len(attachmentContent) > 0 {
			return nil, fmt.Errorf("нельзя одновременно передавать вложение и attachment_file_id")
		}
		name, content, err := c.loadSessionAttachmentForSend(ctx, userId, sessionId, *existingAttachmentFileID)
		if err != nil {
			return nil, err
		}
		attachmentName = name
		attachmentContent = content
		attachmentFileID = existingAttachmentFileID
	} else {
		if len(attachmentContent) > 0 || attachmentName != "" {
			if len(attachmentContent) == 0 || strings.TrimSpace(attachmentName) == "" {
				return nil, fmt.Errorf("вложение передано некорректно")
			}

			if len(attachmentContent) > document.MaxRecommendedAttachmentSizeBytes {
				return nil, fmt.Errorf("размер вложения превышает рекомендуемый максимум: %d байт", document.MaxRecommendedAttachmentSizeBytes)
			}

			if document.IsImageAttachment(attachmentName) {
				if err := document.ValidateImageAttachment(attachmentName, attachmentContent); err != nil {
					return nil, err
				}
			} else if err := document.ValidateAttachment(attachmentName, attachmentContent); err != nil {
				return nil, err
			}
		}

		if len(attachmentContent) > 0 && attachmentName != "" && c.attachmentsSaveDir != "" {
			file, err := c.saveAttachmentAndCreateFile(ctx, userId, sessionId, attachmentName, attachmentContent)
			if err == nil {
				v := file.Id
				attachmentFileID = &v
			} else {
				logger.W("ChatUseCase: вложение: %v", err)
			}
		}
	}

	userMsg := domain.NewMessageWithAttachment(sessionId, userMessage, domain.MessageRoleUser, attachmentFileID)
	if err := c.messageRepo.Create(ctx, userMsg); err != nil {
		logger.E("SendMessage: создание сообщения: %v", err)
		return nil, err
	}

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	messagesForLLM := make([]*domain.Message, 0, len(messages)+2)
	messagesForLLM = append(messagesForLLM, chatSessionSystemMessage(sessionId, settings))
	messagesForLLM = append(messagesForLLM, messages...)
	if len(attachmentContent) > 0 && attachmentName != "" {
		userMsgForLLM := *userMsg
		if document.IsImageAttachment(attachmentName) {
			userMsgForLLM.Content = userMessage
			userMsgForLLM.AttachmentName = attachmentName
			userMsgForLLM.AttachmentContent = attachmentContent
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

	if err := c.hydrateAttachmentsForRunner(ctx, messagesForLLM); err != nil {
		logger.E("SendMessage: подгрузка вложений для раннера: %v", err)
		return nil, err
	}

	if genParams != nil && len(genParams.Tools) > 0 {
		return c.sendMessageWithToolLoop(ctx, userId, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
	}

	assistantMsg := domain.NewMessage(sessionId, "", domain.MessageRoleAssistant)
	if err := c.messageRepo.Create(ctx, assistantMsg); err != nil {
		logger.E("SendMessage: создание черновика ответа: %v", err)
		return nil, err
	}
	messageID := assistantMsg.Id

	responseChan, err := c.llmRepo.SendMessage(ctx, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
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

		for chunk := range responseChan {
			fullResponse.WriteString(chunk)
			select {
			case <-ctx.Done():
				return
			case clientChan <- ChatStreamChunk{Kind: StreamChunkKindText, Text: chunk, MessageID: messageID}:
			}
		}
	}()

	return clientChan, nil
}

func (c *ChatUseCase) SendMessageMulti(ctx context.Context, userId int, sessionId int64, turns []*domain.Message) (chan ChatStreamChunk, error) {
	logger.D("SendMessageMulti: session=%d user=%d turns=%d", sessionId, userId, len(turns))
	session, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		return nil, err
	}

	resolvedModel, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userId, "", session.Model, c.defaultRunnerAddr)
	if err != nil {
		return nil, err
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
			return nil, err
		}
	}

	rawMessages, _, err := c.messageRepo.GetBySessionId(ctx, sessionId, 1, 200)
	if err != nil {
		logger.E("SendMessageMulti: получение сообщений: %v", err)
		return nil, err
	}
	messages := filterHistoryForLLM(rawMessages)

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	messagesForLLM := make([]*domain.Message, 0, len(messages)+1)
	messagesForLLM = append(messagesForLLM, chatSessionSystemMessage(sessionId, settings))
	messagesForLLM = append(messagesForLLM, messages...)

	assistantMsg := domain.NewMessage(sessionId, "", domain.MessageRoleAssistant)
	if err := c.messageRepo.Create(ctx, assistantMsg); err != nil {
		logger.E("SendMessageMulti: черновик ответа: %v", err)
		return nil, err
	}
	messageID := assistantMsg.Id

	stopSequences, timeoutSeconds, genParams := genParamsFromSessionSettings(settings)

	if err := c.hydrateAttachmentsForRunner(ctx, messagesForLLM); err != nil {
		logger.E("SendMessageMulti: подгрузка вложений для раннера: %v", err)
		return nil, err
	}

	responseChan, err := c.llmRepo.SendMessage(ctx, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
	if err != nil {
		logger.E("SendMessageMulti: вызов LLM: %v", err)
		return nil, err
	}

	clientChan := make(chan ChatStreamChunk, 100)
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
			case clientChan <- ChatStreamChunk{Kind: StreamChunkKindText, Text: chunk, MessageID: messageID}:
			}
		}
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

	maxID, err := c.messageRepo.MaxMessageIDInSession(ctx, sessionId)
	if err != nil {
		logger.E("RegenerateAssistantResponse: max id: %v", err)
		return nil, err
	}
	if maxID != assistantMessageID {
		return nil, fmt.Errorf("перегенерировать можно только последнее сообщение в чате")
	}

	resolvedModel, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userId, "", session.Model, c.defaultRunnerAddr)
	if err != nil {
		return nil, err
	}

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	stopSequences, timeoutSeconds, genParams := genParamsFromSessionSettings(settings)
	if genParams != nil && len(genParams.Tools) > 0 {
		return nil, domain.ErrRegenerateToolsNotSupported
	}

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

	if err := c.messageRepo.ResetAssistantForRegenerate(ctx, sessionId, assistantMessageID); err != nil {
		logger.E("RegenerateAssistantResponse: сброс черновика: %v", err)
		return nil, err
	}

	messageID := assistantMessageID

	responseChan, err := c.llmRepo.SendMessage(ctx, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
	if err != nil {
		logger.E("RegenerateAssistantResponse: LLM: %v", err)
		return nil, err
	}

	var fullResponse strings.Builder
	clientChan := make(chan ChatStreamChunk, 100)
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
			case clientChan <- ChatStreamChunk{Kind: StreamChunkKindText, Text: chunk, MessageID: messageID}:
			}
		}
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
	if err := c.messageRepo.UpdateContent(ctx, userMessageID, newContent); err != nil {
		return nil, err
	}

	if c.messageEditRepo != nil {
		_ = c.messageEditRepo.Create(ctx, &domain.MessageEdit{
			SessionId:       sessionId,
			MessageId:       userMessageID,
			EditorUserId:    userId,
			OldContent:      oldContent,
			NewContent:      newContent,
			SoftDeletedFrom: userMessageID,
			SoftDeletedTo:   maxID,
			CreatedAt:       time.Now(),
		})
	}

	if err := c.messageRepo.SoftDeleteRangeAfterID(ctx, sessionId, userMessageID, maxID); err != nil {
		return nil, err
	}

	resolvedModel, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userId, "", session.Model, c.defaultRunnerAddr)
	if err != nil {
		return nil, err
	}

	rawPrefix, err := c.messageRepo.ListMessagesUpToID(ctx, sessionId, userMessageID)
	if err != nil {
		return nil, err
	}
	messages := filterHistoryForLLM(rawPrefix)

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	stopSequences, timeoutSeconds, genParams := genParamsFromSessionSettings(settings)

	messagesForLLM := make([]*domain.Message, 0, len(messages)+1)
	messagesForLLM = append(messagesForLLM, chatSessionSystemMessage(sessionId, settings))
	messagesForLLM = append(messagesForLLM, messages...)

	if err := c.hydrateAttachmentsForRunner(ctx, messagesForLLM); err != nil {
		return nil, err
	}

	if genParams != nil && len(genParams.Tools) > 0 {
		return c.sendMessageWithToolLoop(ctx, userId, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
	}

	assistantMsg := domain.NewMessage(sessionId, "", domain.MessageRoleAssistant)
	if err := c.messageRepo.Create(ctx, assistantMsg); err != nil {
		return nil, err
	}
	messageID := assistantMsg.Id

	responseChan, err := c.llmRepo.SendMessage(ctx, sessionId, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams)
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

		for chunk := range responseChan {
			fullResponse.WriteString(chunk)
			select {
			case <-ctx.Done():
				return
			case clientChan <- ChatStreamChunk{Kind: StreamChunkKindText, Text: chunk, MessageID: messageID}:
			}
		}
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
) (*domain.ChatSessionSettings, error) {
	_, err := c.verifySessionOwnership(ctx, userId, sessionID)
	if err != nil {
		return nil, err
	}
	if stopSequences == nil {
		stopSequences = []string{}
	}
	settings := &domain.ChatSessionSettings{
		SessionID:      sessionID,
		SystemPrompt:   strings.TrimSpace(systemPrompt),
		StopSequences:  stopSequences,
		TimeoutSeconds: timeoutSeconds,
		Temperature:    temperature,
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

	resolvedModel, err := resolveModelForUser(ctx, c.llmRepo, c.preferenceRepo, userId, "", "", c.defaultRunnerAddr)
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

func (c *ChatUseCase) hydrateAttachmentsForRunner(ctx context.Context, messages []*domain.Message) error {
	if len(messages) == 0 {
		return nil
	}

	for _, m := range messages {
		if m == nil {
			continue
		}

		if m.AttachmentFileID == nil || len(m.AttachmentContent) > 0 {
			continue
		}

		if strings.TrimSpace(c.attachmentsSaveDir) == "" {
			return fmt.Errorf("вложение в истории чата (file_id=%d): не задан каталог вложений", *m.AttachmentFileID)
		}

		f, err := c.fileRepo.GetById(ctx, *m.AttachmentFileID)
		if err != nil {
			return fmt.Errorf("файл вложения id=%d: %w", *m.AttachmentFileID, err)
		}

		if f == nil {
			return fmt.Errorf("файл вложения id=%d не найден", *m.AttachmentFileID)
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
			continue
		}

		if err := document.ValidateAttachment(name, data); err != nil {
			return err
		}

		built, err := buildMessageWithFile(name, data, m.Content)
		if err != nil {
			return err
		}

		m.Content = built
	}

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
	fileContent, err := document.ExtractText(attachmentName, attachmentContent)
	if err != nil {
		logger.W("ChatUseCase: извлечение текста из вложения %q: %v", attachmentName, err)
		return "", fmt.Errorf("%w: %v", document.ErrTextExtractionFailed, err)
	}
	fileContent, truncated := document.TruncateExtractedText(fileContent, document.MaxEmbeddedAttachmentTextRunes)

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

	return file.Id, nil
}

func (c *ChatUseCase) GetSessionFile(ctx context.Context, userID int, sessionID int64, fileID int64) (filename string, content []byte, err error) {
	if fileID <= 0 {
		return "", nil, fmt.Errorf("некорректный file_id")
	}

	return c.loadSessionAttachmentForSend(ctx, userID, sessionID, fileID)
}

const (
	sessionFileKindAttachment = "attachment"
	sessionFileKindArtifact   = "artifact"

	sessionArtifactMinTTL        = int64(60)
	sessionArtifactMaxTTL        = int64(7 * 24 * 3600)
	sessionArtifactDefaultTTL    = int64(24 * 3600)
	maxSessionArtifactFileCount  = 200
	maxSessionArtifactTotalBytes = 80 * 1024 * 1024
)

func (c *ChatUseCase) saveAttachmentAndCreateFile(ctx context.Context, userID int, sessionID int64, attachmentName string, content []byte) (*domain.File, error) {
	baseName := filepath.Base(attachmentName)
	if baseName == "" || baseName == "." {
		baseName = "attachment"
	}
	return c.saveFileInSession(ctx, userID, sessionID, baseName, content, sessionFileKindAttachment, nil)
}

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
