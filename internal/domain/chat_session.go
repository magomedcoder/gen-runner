package domain

import "time"

type ChatSession struct {
	Id        int64
	UserId    int
	Title     string
	Model     string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type Message struct {
	Id               int64
	SessionId        int64
	Content          string
	Role             MessageRole
	AttachmentName   string
	AttachmentFileID *int64
	ToolCallID       string
	ToolName         string
	ToolCallsJSON    string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

func NewChatSession(userId int, title string, model string) *ChatSession {
	return &ChatSession{
		UserId:    userId,
		Title:     title,
		Model:     model,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func NewMessage(sessionId int64, content string, role MessageRole) *Message {
	return NewMessageWithAttachment(sessionId, content, role, nil)
}

func NewMessageWithAttachment(sessionId int64, content string, role MessageRole, attachmentFileID *int64) *Message {
	return &Message{
		SessionId:        sessionId,
		Content:          content,
		Role:             role,
		AttachmentFileID: attachmentFileID,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}
