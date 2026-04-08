package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/service"
	"github.com/magomedcoder/gen/pkg/logger"
)

const droppedHistorySummarizerSystem = `Тебе дан фрагмент переписки чата (роли user, assistant, tool). Сожми его в связный пересказ: факты, договорённости, открытые вопросы - чтобы модель могла продолжить без полного текста.
Не более 8 предложений. Без приветствий и метакомментариев. Язык ответа - как у фрагмента.`

func (c *ChatUseCase) summarizeDroppedMessages(ctx context.Context, sessionID int64, chatModel string, chatRunnerAddr string, dropped []*domain.Message) string {
	if c.llmRepo == nil || len(dropped) == 0 {
		return ""
	}

	hints := service.RunnerCoreHints{}
	if c.runnerReg != nil {
		hints = c.runnerReg.AggregateChatHints()
	}
	if c.historySummaryCache != nil {
		c.historySummaryCache.ensureMax(hints.SummaryCacheEntries)
	}

	model := strings.TrimSpace(chatModel)
	if hints.SummaryModel != "" {
		model = hints.SummaryModel
	}

	if model == "" {
		return ""
	}

	body := buildDroppedDialoguePlainText(dropped, hints.SummaryMaxInputRunes)
	if strings.TrimSpace(body) == "" {
		return ""
	}

	cacheKey := historySummaryCacheKey(model, body)
	if c.historySummaryCache != nil {
		if s, ok := c.historySummaryCache.get(cacheKey); ok && strings.TrimSpace(s) != "" {
			logger.V("ChatUseCase: резюме отброшенной истории из кэша")
			return strings.TrimSpace(s)
		}
	}

	sumCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	sys := domain.NewMessage(sessionID, droppedHistorySummarizerSystem, domain.MessageRoleSystem)
	usr := domain.NewMessage(sessionID, body, domain.MessageRoleUser)
	mt := int32(640)
	temp := float32(0.2)
	gp := &domain.GenerationParams{
		MaxTokens:   &mt,
		Temperature: &temp,
	}

	var ch chan domain.LLMStreamChunk
	var err error
	if hints.SummaryRunnerListenAddress != "" && c.runnerPool != nil {
		ch, err = c.runnerPool.SendMessageOnRunner(sumCtx, hints.SummaryRunnerListenAddress, sessionID, model, []*domain.Message{sys, usr}, nil, 90, gp)
	} else if strings.TrimSpace(chatRunnerAddr) != "" && c.runnerPool != nil {
		ch, err = c.runnerPool.SendMessageOnRunner(sumCtx, chatRunnerAddr, sessionID, model, []*domain.Message{sys, usr}, nil, 90, gp)
	} else {
		ch, err = c.llmRepo.SendMessage(sumCtx, sessionID, model, []*domain.Message{sys, usr}, nil, 90, gp)
	}

	if err != nil {
		logger.W("ChatUseCase: резюме отброшенной истории: %v", err)
		return ""
	}

	var b strings.Builder
	for c := range ch {
		b.WriteString(c.Content)
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		logger.W("ChatUseCase: суммаризатор вернул пустой ответ")
		return ""
	}

	if c.historySummaryCache != nil {
		c.historySummaryCache.put(cacheKey, out)
	}

	return out
}
