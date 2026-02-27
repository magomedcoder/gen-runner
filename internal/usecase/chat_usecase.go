package usecase

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
)

type ChatUseCase struct {
	sessionRepo        domain.ChatSessionRepository
	messageRepo        domain.MessageRepository
	fileRepo           domain.FileRepository
	llmRepo            domain.LLMRepository
	attachmentsSaveDir string
}

func NewChatUseCase(
	sessionRepo domain.ChatSessionRepository,
	messageRepo domain.MessageRepository,
	fileRepo domain.FileRepository,
	llmRepo domain.LLMRepository,
	attachmentsSaveDir string,
) *ChatUseCase {
	return &ChatUseCase{
		sessionRepo:        sessionRepo,
		messageRepo:        messageRepo,
		fileRepo:           fileRepo,
		llmRepo:            llmRepo,
		attachmentsSaveDir: attachmentsSaveDir,
	}
}

func (c *ChatUseCase) verifySessionOwnership(ctx context.Context, userId int, sessionID string) (*domain.ChatSession, error) {
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

func (c *ChatUseCase) SendMessage(ctx context.Context, userId int, sessionId string, model string, userMessage string, attachmentName string, attachmentContent []byte) (chan string, string, error) {
	logger.D("SendMessage: session=%s user=%d model=%s", sessionId, userId, model)
	_, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		logger.W("SendMessage: сессия не принадлежит пользователю: %v", err)
		return nil, "", err
	}

	messages, _, err := c.messageRepo.GetBySessionId(ctx, sessionId, 1, 100)
	if err != nil {
		logger.E("SendMessage: получение сообщений: %v", err)
		return nil, "", err
	}

	var attachmentFileID string
	if len(attachmentContent) > 0 && attachmentName != "" && c.attachmentsSaveDir != "" {
		file, _, err := c.saveAttachmentAndCreateFile(sessionId, attachmentName, attachmentContent)
		if err == nil {
			attachmentFileID = file.Id
			if err := c.fileRepo.Create(ctx, file); err != nil {
				logger.W("ChatUseCase: не удалось сохранить запись файла: %v", err)
				attachmentFileID = ""
			}
		}
	}

	userMsg := domain.NewMessageWithAttachment(sessionId, userMessage, domain.MessageRoleUser, attachmentFileID)
	if err := c.messageRepo.Create(ctx, userMsg); err != nil {
		logger.E("SendMessage: создание сообщения: %v", err)
		return nil, "", err
	}

	messagesForLLM := make([]*domain.Message, 0, len(messages)+1)
	messagesForLLM = append(messagesForLLM, messages...)
	if len(attachmentContent) > 0 && attachmentName != "" {
		fullContent := buildMessageWithFile(attachmentName, attachmentContent, userMessage)
		userMsgForLLM := *userMsg
		userMsgForLLM.Content = fullContent
		messagesForLLM = append(messagesForLLM, &userMsgForLLM)
	} else {
		messagesForLLM = append(messagesForLLM, userMsg)
	}

	responseChan, err := c.llmRepo.SendMessage(ctx, sessionId, model, messagesForLLM)
	if err != nil {
		logger.E("SendMessage: вызов LLM: %v", err)
		return nil, "", err
	}
	logger.V("SendMessage: стрим от LLM запущен session=%s", sessionId)

	assistantMsg := domain.NewMessage(sessionId, "", domain.MessageRoleAssistant)
	messageId := assistantMsg.Id
	var fullResponse strings.Builder

	clientChan := make(chan string, 100)
	go func() {
		defer func() {
			assistantMsg.Content = fullResponse.String()
			c.messageRepo.Create(context.Background(), assistantMsg)
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

	return clientChan, messageId, nil
}

func (c *ChatUseCase) CreateSession(ctx context.Context, userId int, title string, model string) (*domain.ChatSession, error) {
	session := domain.NewChatSession(userId, title, model)
	if err := c.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

func (c *ChatUseCase) GetSession(ctx context.Context, userId int, sessionID string) (*domain.ChatSession, error) {
	return c.verifySessionOwnership(ctx, userId, sessionID)
}

func (c *ChatUseCase) GetSessions(ctx context.Context, userId int, page, pageSize int32) ([]*domain.ChatSession, int32, error) {
	return c.sessionRepo.GetByUserId(ctx, userId, page, pageSize)
}

func (c *ChatUseCase) GetSessionMessages(ctx context.Context, userId int, sessionId string, page, pageSize int32) ([]*domain.Message, int32, error) {
	_, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		return nil, 0, err
	}

	return c.messageRepo.GetBySessionId(ctx, sessionId, page, pageSize)
}

func (c *ChatUseCase) DeleteSession(ctx context.Context, userId int, sessionID string) error {
	_, err := c.verifySessionOwnership(ctx, userId, sessionID)
	if err != nil {
		return err
	}

	return c.sessionRepo.Delete(ctx, sessionID)
}

func (c *ChatUseCase) UpdateSessionTitle(ctx context.Context, userId int, sessionId string, title string) (*domain.ChatSession, error) {
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

func (c *ChatUseCase) UpdateSessionModel(ctx context.Context, userId int, sessionId string, model string) (*domain.ChatSession, error) {
	session, err := c.verifySessionOwnership(ctx, userId, sessionId)
	if err != nil {
		return nil, err
	}

	session.Model = model
	if err := c.sessionRepo.Update(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
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

func (c *ChatUseCase) saveAttachmentAndCreateFile(sessionId, attachmentName string, content []byte) (*domain.File, string, error) {
	baseName := filepath.Base(attachmentName)
	if baseName == "" || baseName == "." {
		baseName = "attachment"
	}
	dir := filepath.Join(c.attachmentsSaveDir, sessionId)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, "", err
	}
	file := domain.NewFile(baseName, "", int64(len(content)), "")
	storageName := file.Id + "_" + baseName
	storagePath := filepath.Join(dir, storageName)
	file.StoragePath = storagePath
	if err := os.WriteFile(storagePath, content, 0644); err != nil {
		return nil, "", err
	}
	return file, storagePath, nil
}
