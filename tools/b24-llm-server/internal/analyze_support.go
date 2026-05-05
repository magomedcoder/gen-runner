package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/service"
)

const OutputFormatTaskAnalysis = "task_analysis"

const (
	maxDescriptionRunes = 12000
	maxCommentRunes     = 4000
)

type analysisStep struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

type analysisRisk struct {
	Level      string `json:"level"`
	Title      string `json:"title"`
	Mitigation string `json:"mitigation"`
}

type taskAnalysis struct {
	Summary            string         `json:"summary"`
	Steps              []analysisStep `json:"steps"`
	Risks              []analysisRisk `json:"risks"`
	AcceptanceCriteria []string       `json:"acceptance_criteria"`
}

func systemPromptForAnalysisMode(mode string) string {
	base := "Ты аналитик задач Bitrix24. Отвечай структурировано и по делу. Используй контекст задачи и комментариев."
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "summarize":
		return base + " Режим: краткое резюме (3-5 предложений) для руководителя."
	case "plan":
		return base + " Режим: план работ - нумерованный список шагов, порядок и зависимости."
	case "risks":
		return base + " Режим: риски и блокеры - список с пояснением серьёзности."
	case "clarify":
		return base + " Режим: уточняющие вопросы постановщику - маркированный список."
	case "acceptance":
		return base + " Режим: критерии приёмки - измеримые пункты."
	case "draft_comment":
		return "Ты помощник по задачам Bitrix24. Напиши один связный комментарий от лица исполнителя: вежливо, по существу, готовый к публикации."
	case "recap":
		return base + " Режим: пересказ для нового участника - 5-7 маркеров: решения, блокеры, договорённости, открытые вопросы; только факты из контекста и комментариев."
	default:
		return base + " Отмечай риски, шаги и уточняющие вопросы, где уместно."
	}
}

func effectiveUserPrompt(req AnalyzeRequest) string {
	p := strings.TrimSpace(req.Prompt)
	if p != "" {
		return p
	}

	switch strings.ToLower(strings.TrimSpace(req.AnalysisMode)) {
	case "summarize":
		return "Сделай краткое резюме задачи для руководителя (3-5 предложений)."
	case "plan":
		return "Составь пошаговый план работ (WBS) с учётом контекста и комментариев."
	case "risks":
		return "Выдели риски, блокеры и неясности постановки; предложи смягчение рисков."
	case "clarify":
		return "Сформулируй уточняющие вопросы постановщику, чтобы закрыть пробелы в ТЗ."
	case "acceptance":
		return "Предложи чёткие критерии приёмки (acceptance criteria) по текущему описанию и обсуждению."
	case "draft_comment":
		return "Подготовь нейтральный черновик комментария к задаче для публикации в Bitrix24."
	case "recap":
		return "Сделай пересказ переписки по задаче для человека, который только подключается: 5-7 коротких маркированных пунктов - что уже решили, на чём застряли, какие договорённости и открытые вопросы; без воды."
	default:
		return ""
	}
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}

	if utf8.RuneCountInString(s) <= max {
		return s
	}

	r := []rune(s)
	return string(r[:max]) + "\n… (текст усечён)"
}

func buildTaskContext(req AnalyzeRequest) string {
	var b strings.Builder
	b.WriteString("Контекст задачи из Bitrix24:\n")
	b.WriteString(fmt.Sprintf("- ID: %s\n", strings.TrimSpace(req.TaskID)))
	b.WriteString(fmt.Sprintf("- Название: %s\n", strings.TrimSpace(req.TaskTitle)))
	b.WriteString(fmt.Sprintf("- Статус: %s\n", strings.TrimSpace(req.TaskStatus)))
	b.WriteString(fmt.Sprintf("- Срок: %s\n", strings.TrimSpace(req.TaskDeadline)))
	b.WriteString(fmt.Sprintf("- Приоритет: %s\n", strings.TrimSpace(req.TaskPriority)))
	b.WriteString(fmt.Sprintf("- Группа/проект (ID): %s\n", strings.TrimSpace(req.TaskGroupID)))
	b.WriteString(fmt.Sprintf("- Исполнитель (ID): %s\n", strings.TrimSpace(req.TaskAssignee)))
	b.WriteString(fmt.Sprintf("- Постановщик (ID): %s\n", strings.TrimSpace(req.TaskCreatedBy)))
	b.WriteString(fmt.Sprintf("- Соисполнители (ID): %s\n", strings.TrimSpace(req.TaskAccomplices)))
	b.WriteString(fmt.Sprintf("- Наблюдатели (ID): %s\n", strings.TrimSpace(req.TaskAuditors)))
	b.WriteString(fmt.Sprintf("- Родительская задача (ID): %s\n", strings.TrimSpace(req.TaskParentID)))
	b.WriteString(fmt.Sprintf("- Оценка времени (сек, как в Bitrix): %s\n", strings.TrimSpace(req.TaskTimeEstimate)))
	b.WriteString(fmt.Sprintf("- Учтённое время в логах (сек): %s\n", strings.TrimSpace(req.TaskTimeSpent)))
	b.WriteString(fmt.Sprintf("- Теги: %s\n", strings.TrimSpace(req.TaskTags)))
	desc := truncateRunes(strings.TrimSpace(req.TaskDescription), maxDescriptionRunes)
	b.WriteString(fmt.Sprintf("- Описание:\n%s\n", desc))
	b.WriteString("\nКомментарии:\n")

	if len(req.Comments) == 0 {
		b.WriteString("- Комментариев нет.\n")
	} else {
		for _, c := range req.Comments {
			text := truncateRunes(strings.TrimSpace(c.Text), maxCommentRunes)
			b.WriteString(fmt.Sprintf("- [%s] %s: %s\n",
				strings.TrimSpace(c.Time),
				strings.TrimSpace(c.Author),
				text,
			))
		}
	}

	b.WriteString("\nЧек-лист:\n")
	if len(req.Checklist) == 0 {
		b.WriteString("- Пунктов нет или данные не переданы.\n")
	} else {
		for _, it := range req.Checklist {
			done := strings.TrimSpace(it.Done)
			if done == "" {
				done = "?"
			}
			b.WriteString(fmt.Sprintf("- [%s] %s\n", done, strings.TrimSpace(it.Title)))
		}
	}

	b.WriteString("\nПодзадачи:\n")
	if len(req.Subtasks) == 0 {
		b.WriteString("- Подзадач нет или данные не переданы.\n")
	} else {
		for _, st := range req.Subtasks {
			b.WriteString(fmt.Sprintf("- #%s (%s) %s\n",
				strings.TrimSpace(st.ID),
				strings.TrimSpace(st.Status),
				strings.TrimSpace(st.Title),
			))
		}
	}

	b.WriteString("\nЗависимости - сначала должны быть выполнены (предшественники):\n")
	if len(req.DependenciesPredecessors) == 0 {
		b.WriteString("- Нет или данные не переданы.\n")
	} else {
		for _, st := range req.DependenciesPredecessors {
			b.WriteString(fmt.Sprintf("- #%s (%s) %s\n",
				strings.TrimSpace(st.ID),
				strings.TrimSpace(st.Status),
				strings.TrimSpace(st.Title),
			))
		}
	}

	b.WriteString("\nЗависимости - от текущей зависят (последователи):\n")
	if len(req.DependenciesSuccessors) == 0 {
		b.WriteString("- Нет или данные не переданы.\n")
	} else {
		for _, st := range req.DependenciesSuccessors {
			b.WriteString(fmt.Sprintf("- #%s (%s) %s\n",
				strings.TrimSpace(st.ID),
				strings.TrimSpace(st.Status),
				strings.TrimSpace(st.Title),
			))
		}
	}

	b.WriteString("\nПользовательские поля (UF):\n")
	if len(req.TaskUserFields) == 0 {
		b.WriteString("- Нет или данные не переданы.\n")
	} else {
		for _, uf := range req.TaskUserFields {
			b.WriteString(fmt.Sprintf("- %s: %s\n",
				strings.TrimSpace(uf.Field),
				strings.TrimSpace(uf.Value),
			))
		}
	}

	b.WriteString("\nВложения задачи (файлы в Диске, по данным Bitrix):\n")
	if len(req.TaskAttachments) == 0 {
		b.WriteString("- Нет или данные не переданы.\n")
	} else {
		for _, a := range req.TaskAttachments {
			name := strings.TrimSpace(a.Name)
			if name == "" {
				name = "(имя недоступно)"
			}
			b.WriteString(fmt.Sprintf("- id=%s - %s\n", strings.TrimSpace(a.ID), name))
		}
	}

	return b.String()
}

func useStructuredTaskAnalysis(req AnalyzeRequest) bool {
	return strings.TrimSpace(req.OutputFormat) == OutputFormatTaskAnalysis
}

func structuredTaskAnalysisSystemAddon() string {
	return `Формат ответа: только один JSON-объект (без Markdown, без текста до или после).
Схема полей:
- "summary": string — краткий вывод (можно пустую строку, если всё в разделах ниже).
- "steps": массив { "title": string, "detail": string } — шаги плана; "title" не пустой.
- "risks": массив { "level": string, "title": string, "mitigation": string } — риски; level одно из: low, medium, high (или низкий/средний/высокий — будут нормализованы).
- "acceptance_criteria": массив строк — критерии приёмки.
Требование: хотя бы один из разделов (непустой summary, непустой steps, непустой risks или непустой acceptance_criteria) должен содержать содержательные данные.`
}

func stripMarkdownJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}

	rest := s
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:]
	} else {
		return strings.TrimSpace(s)
	}

	if end := strings.Index(rest, "```"); end >= 0 {
		return strings.TrimSpace(rest[:end])
	}

	return strings.TrimSpace(rest)
}

func extractJSONObjectFromModelText(s string) ([]byte, error) {
	s = stripMarkdownJSONFence(s)
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i < 0 || j <= i {
		return nil, fmt.Errorf("нет JSON-объекта в ответе")
	}

	return []byte(s[i : j+1]), nil
}

func normalizeRiskLevel(level string) (string, error) {
	l := strings.ToLower(strings.TrimSpace(level))
	switch l {
	case "low", "medium", "high":
		return l, nil
	case "низкий", "низк.":
		return "low", nil
	case "средний", "средн.":
		return "medium", nil
	case "высокий", "высок.":
		return "high", nil
	default:
		return "", fmt.Errorf("недопустимый level риска: %q", level)
	}
}

func validateAndPrettyTaskAnalysis(rawJSON []byte) (pretty string, err error) {
	var v taskAnalysis
	if err := json.Unmarshal(rawJSON, &v); err != nil {
		return "", err
	}

	hasContent := strings.TrimSpace(v.Summary) != ""
	for _, st := range v.Steps {
		if strings.TrimSpace(st.Title) != "" {
			hasContent = true
			break
		}
	}

	if !hasContent {
		for _, r := range v.Risks {
			if strings.TrimSpace(r.Title) != "" {
				hasContent = true
				break
			}
		}
	}

	if !hasContent {
		for _, ac := range v.AcceptanceCriteria {
			if strings.TrimSpace(ac) != "" {
				hasContent = true
				break
			}
		}
	}

	if !hasContent {
		return "", fmt.Errorf("все разделы пустые")
	}

	for i := range v.Steps {
		if strings.TrimSpace(v.Steps[i].Title) == "" {
			return "", fmt.Errorf("шаг %d без title", i+1)
		}
	}

	for i := range v.Risks {
		nl, err := normalizeRiskLevel(v.Risks[i].Level)
		if err != nil {
			return "", fmt.Errorf("риск %d: %w", i+1, err)
		}

		v.Risks[i].Level = nl
		if strings.TrimSpace(v.Risks[i].Title) == "" {
			return "", fmt.Errorf("риск %d без title", i+1)
		}
	}

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}

	return string(out), nil
}

func truncateForRepair(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}

	if utf8.RuneCountInString(s) <= max {
		return s
	}

	r := []rune(s)
	return string(r[:max]) + "…"
}

func collectLLMText(ch <-chan domain.LLMStreamChunk) string {
	var b strings.Builder
	for chunk := range ch {
		b.WriteString(chunk.Content)
		b.WriteString(chunk.ReasoningContent)
	}

	return strings.TrimSpace(b.String())
}

func finalizeStructuredTaskAnalysis(ctx context.Context,llm *service.LLMRunnerService,	model string, genParams *domain.GenerationParams,	stopSequences []string,baseMessages []*domain.Message,firstRaw string) (string, error) {
	raw := strings.TrimSpace(firstRaw)
	var parseErr error
	b, exErr := extractJSONObjectFromModelText(raw)
	if exErr != nil {
		parseErr = exErr
	} else {
		pretty, vErr := validateAndPrettyTaskAnalysis(b)
		if vErr == nil {
			return pretty, nil
		}
		parseErr = vErr
	}

	repair := fmt.Sprintf(
		"Исправь ответ: нужен ровно один JSON-объект по согласованной схеме (summary, steps, risks, acceptance_criteria), без Markdown и без текста вокруг.\n"+
			"Ошибка разбора: %v\n"+
			"Предыдущий ответ:\n%s",
		parseErr,
		truncateForRepair(raw, 12000),
	)

	msgs := append(append(cloneMessages(baseMessages),
		&domain.Message{
			Role:    domain.MessageRoleAssistant,
			Content: raw,
		},
	), &domain.Message{
		Role:    domain.MessageRoleUser,
		Content: repair,
	})

	ch2, err := llm.SendMessage(ctx, 0, model, msgs, stopSequences, 0, genParams)
	if err != nil {
		return "", fmt.Errorf("повторный запрос: %w", err)
	}

	second := collectLLMText(ch2)
	b2, err := extractJSONObjectFromModelText(second)
	if err != nil {
		return "", fmt.Errorf("повтор: %w", err)
	}

	pretty, err := validateAndPrettyTaskAnalysis(b2)
	if err != nil {
		return "", fmt.Errorf("повтор: валидация: %w", err)
	}

	return pretty, nil
}

func cloneMessages(in []*domain.Message) []*domain.Message {
	out := make([]*domain.Message, len(in))
	for i, m := range in {
		if m == nil {
			continue
		}

		out[i] = &domain.Message{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	return out
}
