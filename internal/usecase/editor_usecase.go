package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/magomedcoder/gen/api/pb/app/editorpb"
	"github.com/magomedcoder/gen/internal/domain"
)

type EditorUseCase struct {
	llmRepo     domain.LLMRepository
	historyRepo domain.EditorHistoryRepository
	runners     domain.RunnerRepository
}

func NewEditorUseCase(
	llmRepo domain.LLMRepository,
	historyRepo domain.EditorHistoryRepository,
	runners domain.RunnerRepository,
) *EditorUseCase {
	return &EditorUseCase{
		llmRepo:     llmRepo,
		historyRepo: historyRepo,
		runners:     runners,
	}
}

func (e *EditorUseCase) Transform(
	ctx context.Context,
	userID int,
	text string,
	t editorpb.TransformType,
	preserveMarkdown bool,
) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("пустой текст")
	}
	resolvedModel, err := resolveModelForUser(ctx, e.llmRepo, "", "")
	if err != nil {
		return "", err
	}

	sessionID := time.Now().UnixNano()
	system := buildEditorSystemPrompt(t, preserveMarkdown)

	messages := []*domain.Message{
		domain.NewMessage(sessionID, system, domain.MessageRoleSystem),
		domain.NewMessage(sessionID, wrapUserText(text), domain.MessageRoleUser),
	}

	ch, err := e.llmRepo.SendMessage(ctx, sessionID, resolvedModel, messages, nil, 0, nil)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for c := range ch {
		b.WriteString(c.Content)
	}

	return strings.TrimSpace(b.String()), nil
}

func (e *EditorUseCase) SaveHistory(ctx context.Context, userID int, runner string, text string) error {
	var runnerID *int64
	if strings.TrimSpace(runner) != "" {
		rid, ok, err := e.runners.FindIDByListenAddress(ctx, runner)
		if err != nil {
			return err
		}
		if ok {
			runnerID = &rid
		}
	}
	return e.historyRepo.Save(ctx, userID, runnerID, text)
}

func wrapUserText(text string) string {
	return "Текст:\n\n```\n" + text + "\n```"
}

func buildEditorSystemPrompt(t editorpb.TransformType, preserveMarkdown bool) string {
	action := "улучши текст"
	switch t {
	case editorpb.TransformType_TRANSFORM_TYPE_FIX:
		action = "исправь орфографию, пунктуацию и грамматику"
	case editorpb.TransformType_TRANSFORM_TYPE_IMPROVE:
		action = "улучши текст: сделай яснее, логичнее и читабельнее, не меняя смысл"
	case editorpb.TransformType_TRANSFORM_TYPE_BEAUTIFY:
		action = "сделай текст более красивым и выразительным, сохраняя смысл"
	case editorpb.TransformType_TRANSFORM_TYPE_PARAPHRASE:
		action = "перефразируй (другими словами), сохраняя смысл"
	case editorpb.TransformType_TRANSFORM_TYPE_SHORTEN:
		action = "сократи текст, сохранив ключевой смысл и факты"
	case editorpb.TransformType_TRANSFORM_TYPE_SIMPLIFY:
		action = "упрости текст: сделай проще и понятнее, без потери смысла"
	case editorpb.TransformType_TRANSFORM_TYPE_MAKE_COMPLEX:
		action = "сделай текст более сложным/профессиональным: добавь точности и терминов, сохраняя смысл"
	case editorpb.TransformType_TRANSFORM_TYPE_MORE_FORMAL:
		action = "перепиши в более формальном стиле"
	case editorpb.TransformType_TRANSFORM_TYPE_MORE_CASUAL:
		action = "перепиши в разговорном стиле"
	default:
		action = "улучши текст"
	}

	formatRule := "Сохраняй переносы строк и структуру по смыслу."
	if preserveMarkdown {
		formatRule = "Сохраняй Markdown/разметку, списки и переносы строк (если они есть)."
	}

	return fmt.Sprintf(
		"Ты - редактор текста. Задача: %s.\n"+
			"Правила:\n"+
			"- Верни ТОЛЬКО итоговый отредактированный текст, без пояснений.\n"+
			"- Язык результата: тот же, что и исходный текст; если язык неочевиден, используй русский.\n"+
			"- Сохраняй смысл; не добавляй новых фактов.\n"+
			"- Имена, числа, даты и сущности не меняй (кроме явных опечаток).\n"+
			"- %s\n",
		action, formatRule,
	)
}
