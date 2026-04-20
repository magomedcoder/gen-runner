package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
)

func (c *ChatUseCase) RegenerateAssistantResponse(ctx context.Context, userId int, sessionId int64, assistantMessageID int64) (chan ChatStreamChunk, error) {
	logger.D("RegenerateAssistantResponse: сессия=%d пользователь=%d сообщение_ассистента=%d", sessionId, userId, assistantMessageID)
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
		logger.E("RegenerateAssistantResponse: макс. id сообщения: %v", err)
		return nil, err
	}

	if maxID != assistantMessageID {
		return nil, fmt.Errorf("перегенерировать можно только последнее сообщение в чате")
	}

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	if len(parseToolsJSON(settings.ToolsJSON)) > 0 {
		return nil, domain.ErrRegenerateToolsNotSupported
	}

	if settings.MCPEnabled && len(c.mcpEffectiveServerIDs(ctx, userId, settings)) > 0 {
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
			if r := recover(); r != nil {
				logger.E("RegenerateAssistantResponse panic: session_id=%d user_id=%d panic=%v", sessionId, userId, r)
				select {
				case <-ctx.Done():
				case clientChan <- ChatStreamChunk{Kind: StreamChunkKindText, Text: "внутренняя ошибка обработки ответа"}:
				}
			}
		}()
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
			case clientChan <- ChatStreamChunk{
				Kind: StreamChunkKindNotice,
				Text: HistoryTruncatedClientNotice,
			}:
			}
		}

		forwardLLMStreamChunks(ctx, clientChan, messageID, responseChan, &fullResponse)
	}()

	return clientChan, nil
}

func (c *ChatUseCase) ContinueAssistantResponse(ctx context.Context, userId int, sessionId int64, assistantMessageID int64) (chan ChatStreamChunk, error) {
	logger.D("ContinueAssistantResponse: сессия=%d пользователь=%d сообщение_ассистента=%d", sessionId, userId, assistantMessageID)
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
		logger.E("ContinueAssistantResponse: макс. id сообщения: %v", err)
		return nil, err
	}

	if maxID != assistantMessageID {
		return nil, fmt.Errorf("продолжить можно только последнее сообщение в чате")
	}

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	if len(parseToolsJSON(settings.ToolsJSON)) > 0 {
		return nil, domain.ErrRegenerateToolsNotSupported
	}

	if settings.MCPEnabled && len(c.mcpEffectiveServerIDs(ctx, userId, settings)) > 0 {
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
			if r := recover(); r != nil {
				logger.E("ContinueAssistantResponse panic: session_id=%d user_id=%d panic=%v", sessionId, userId, r)
				select {
				case <-ctx.Done():
				case clientChan <- ChatStreamChunk{Kind: StreamChunkKindText, Text: "внутренняя ошибка обработки ответа"}:
				}
			}
		}()
		defer func() {
			_ = c.messageRepo.UpdateContent(context.Background(), messageID, existingContent+newPart.String())
		}()
		defer close(clientChan)

		if contHistoryNotice {
			select {
			case <-ctx.Done():
				return
			case clientChan <- ChatStreamChunk{
				Kind: StreamChunkKindNotice,
				Text: HistoryTruncatedClientNotice,
			}:
			}
		}

		forwardLLMStreamChunks(ctx, clientChan, messageID, responseChan, &newPart)
	}()

	return clientChan, nil
}

func (c *ChatUseCase) EditUserMessageAndContinue(ctx context.Context, userId int, sessionId int64, userMessageID int64, newContent string) (chan ChatStreamChunk, error) {
	logger.D("EditUserMessageAndContinue: сессия=%d пользователь=%d сообщение_пользователя=%d", sessionId, userId, userMessageID)
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
	c.injectWebSearchAndMCP(ctx, genParams, settings, userId, sessionId)

	messagesForLLM := make([]*domain.Message, 0, len(messages)+1)
	messagesForLLM = append(messagesForLLM, c.llmChatSystemMessage(ctx, sessionId, settings, userId))
	messagesForLLM = append(messagesForLLM, messages...)

	if err := c.hydrateAttachmentsForRunner(ctx, messagesForLLM); err != nil {
		return nil, err
	}

	var editHistoryNotice bool
	messagesForLLM, editHistoryNotice = c.capLLMHistoryTokens(ctx, messagesForLLM, 1, sessionId, resolvedModel, runnerAddr, true)

	if genParams != nil && len(genParams.Tools) > 0 {
		return c.sendMessageWithToolLoop(ctx, userId, sessionId, runnerAddr, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams, editHistoryNotice, nil)
	}
	if settings != nil {
		toolsCount := 0
		if genParams != nil {
			toolsCount = len(genParams.Tools)
		}
		logger.W("EditUserMessageAndContinue: tool-loop disabled session_id=%d tools=%d mcp_enabled=%t mcp_server_ids=%v web_search_enabled=%t", sessionId, toolsCount, settings.MCPEnabled, settings.MCPServerIDs, settings.WebSearchEnabled)
	} else {
		logger.W("EditUserMessageAndContinue: tool-loop disabled session_id=%d settings=nil tools=0", sessionId)
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
			if r := recover(); r != nil {
				logger.E("EditUserMessageAndContinue panic: session_id=%d user_id=%d panic=%v", sessionId, userId, r)
				select {
				case <-ctx.Done():
				case clientChan <- ChatStreamChunk{Kind: StreamChunkKindText, Text: "внутренняя ошибка обработки ответа"}:
				}
			}
		}()
		defer func() {
			_ = c.messageRepo.UpdateContent(context.Background(), messageID, fullResponse.String())
		}()
		defer close(clientChan)

		if editHistoryNotice {
			select {
			case <-ctx.Done():
				return
			case clientChan <- ChatStreamChunk{
				Kind: StreamChunkKindNotice,
				Text: HistoryTruncatedClientNotice,
			}:
			}
		}

		forwardLLMStreamChunks(ctx, clientChan, messageID, responseChan, &fullResponse)
	}()

	return clientChan, nil
}
