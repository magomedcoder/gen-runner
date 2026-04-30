package internal

import (
	"fmt"
	"strings"
)

type AnalyzeRequest struct {
	TaskID          string        `json:"task_id"`
	TaskTitle       string        `json:"task_title"`
	TaskDescription string        `json:"task_description"`
	TaskStatus      string        `json:"task_status"`
	TaskDeadline    string        `json:"task_deadline"`
	TaskAssignee    string        `json:"task_assignee"`
	Comments        []TaskComment `json:"comments"`
	History         []ChatMessage `json:"history"`
	Prompt          string        `json:"prompt"`
}

type TaskComment struct {
	Author string `json:"author"`
	Text   string `json:"text"`
	Time   string `json:"time"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnalyzeResponse struct {
	Message string `json:"message"`
}

func buildTaskContext(req AnalyzeRequest) string {
	var b strings.Builder
	b.WriteString("Контекст задачи из Bitrix24:\n")
	b.WriteString(fmt.Sprintf("- ID: %s\n", strings.TrimSpace(req.TaskID)))
	b.WriteString(fmt.Sprintf("- Название: %s\n", strings.TrimSpace(req.TaskTitle)))
	b.WriteString(fmt.Sprintf("- Статус: %s\n", strings.TrimSpace(req.TaskStatus)))
	b.WriteString(fmt.Sprintf("- Срок: %s\n", strings.TrimSpace(req.TaskDeadline)))
	b.WriteString(fmt.Sprintf("- Исполнитель: %s\n", strings.TrimSpace(req.TaskAssignee)))
	b.WriteString(fmt.Sprintf("- Описание:\n%s\n", strings.TrimSpace(req.TaskDescription)))
	b.WriteString("\nКомментарии:\n")

	if len(req.Comments) == 0 {
		b.WriteString("- Комментариев нет.\n")
	} else {
		for _, c := range req.Comments {
			b.WriteString(fmt.Sprintf("- [%s] %s: %s\n",
				strings.TrimSpace(c.Time),
				strings.TrimSpace(c.Author),
				strings.TrimSpace(c.Text),
			))
		}
	}

	return b.String()
}
