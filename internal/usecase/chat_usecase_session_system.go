package usecase

import (
	"context"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
)

const defaultResponseLanguagePrompt = "Язык ответа: отвечай на том же языке, что и последнее сообщение пользователя в этом запросе. Если язык нельзя определить (например, только код, числа или нейтральные символы), отвечай по-русски."

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
