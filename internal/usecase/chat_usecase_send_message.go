package usecase

import (
	"context"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
)

func (c *ChatUseCase) SendMessage(ctx context.Context, userId int, sessionId int64, userMessage string, attachmentFileIDs []int64, fileRAG *SendMessageFileRAGOptions) (chan ChatStreamChunk, error) {
	logger.D("SendMessage: сессия=%d пользователь=%d", sessionId, userId)
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

	normalizedAttachmentFileIDs, err := normalizeAttachmentFileIDsForModel(attachmentFileIDs)
	if err != nil {
		return nil, err
	}

	if err := validateFileRAGOptions(fileRAG, normalizedAttachmentFileIDs); err != nil {
		return nil, err
	}

	attachmentNames := make([]string, 0, len(normalizedAttachmentFileIDs))
	attachmentContents := make([][]byte, 0, len(normalizedAttachmentFileIDs))
	for _, fid := range normalizedAttachmentFileIDs {
		name, content, err := c.loadSessionAttachmentForSend(ctx, userId, sessionId, fid)
		if err != nil {
			return nil, err
		}
		attachmentNames = append(attachmentNames, name)
		attachmentContents = append(attachmentContents, content)
	}

	var storedAttachmentFileID *int64
	if len(normalizedAttachmentFileIDs) > 0 {
		v := normalizedAttachmentFileIDs[0]
		storedAttachmentFileID = &v
	}
	userMsg := domain.NewMessageWithAttachment(sessionId, userMessage, domain.MessageRoleUser, storedAttachmentFileID)
	if err := c.messageRepo.Create(ctx, userMsg); err != nil {
		logger.E("SendMessage: создание сообщения: %v", err)
		return nil, err
	}

	settings, _ := c.sessionSettingsRepo.GetBySessionID(ctx, sessionId)
	messagesForLLM, ragStream, err := c.buildSendPromptAssembly(
		ctx,
		sendPromptAssemblyInput{
			sessionID:                sessionId,
			userID:                   userId,
			resolvedModel:            resolvedModel,
			settings:                 settings,
			history:                  messages,
			userMessage:              userMessage,
			userMsg:                  userMsg,
			attachmentFileIDs:        normalizedAttachmentFileIDs,
			attachmentNames:          attachmentNames,
			attachmentContents:       attachmentContents,
			fileRAG:                  fileRAG,
			preferFullDocumentIfFits: c.preferFullDocumentWhenFits,
		},
	)
	if err != nil {
		return nil, err
	}

	stopSequences, timeoutSeconds, genParams := genParamsFromSessionSettings(settings)
	c.injectWebSearchAndMCP(ctx, genParams, settings, userId, sessionId)

	if err := c.hydrateAttachmentsForRunner(ctx, messagesForLLM); err != nil {
		logger.E("SendMessage: подгрузка вложений для раннера: %v", err)
		return nil, err
	}
	var historyNotice bool
	messagesForLLM, historyNotice = c.capLLMHistoryTokens(ctx, messagesForLLM, 1, sessionId, resolvedModel, runnerAddr, true)

	if genParams != nil && len(genParams.Tools) > 0 {
		return c.sendMessageWithToolLoop(ctx, userId, sessionId, runnerAddr, resolvedModel, messagesForLLM, stopSequences, timeoutSeconds, genParams, historyNotice, ragStream)
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
	logger.V("SendMessage: стрим от LLM запущен сессия=%d", sessionId)

	var fullResponse strings.Builder
	clientChan := make(chan ChatStreamChunk, 100)
	go func() {
		defer func() {
			_ = c.messageRepo.UpdateContent(context.Background(), messageID, fullResponse.String())
		}()
		defer close(clientChan)

		if ragStream != nil {
			select {
			case <-ctx.Done():
				return
			case clientChan <- ragStream.asChunk():
			}
		}

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
