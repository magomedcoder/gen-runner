package domain

import (
	"context"
)

type UserRepository interface {
	Create(ctx context.Context, user *User) error

	GetById(ctx context.Context, id int) (*User, error)

	GetByUsername(ctx context.Context, username string) (*User, error)

	Update(ctx context.Context, user *User) error

	List(ctx context.Context, page, pageSize int32) ([]*User, int32, error)

	UpdateLastVisitedAt(ctx context.Context, userID int) error
}

type TokenRepository interface {
	Create(ctx context.Context, token *Token) error

	GetByToken(ctx context.Context, token string) (*Token, error)

	DeleteByToken(ctx context.Context, token string) error

	DeleteByUserId(ctx context.Context, userId int, tokenType TokenType) error
}

type ChatSessionRepository interface {
	Create(ctx context.Context, session *ChatSession) error

	GetById(ctx context.Context, id int64) (*ChatSession, error)

	GetByUserId(ctx context.Context, userID int, page, pageSize int32) ([]*ChatSession, int32, error)

	Update(ctx context.Context, session *ChatSession) error

	Delete(ctx context.Context, id int64) error
}

type ChatPreferenceRepository interface {
	GetSelectedRunner(ctx context.Context, userID int) (string, error)

	SetSelectedRunner(ctx context.Context, userID int, runner string) error

	GetDefaultRunnerModel(ctx context.Context, userID int, runner string) (string, error)

	SetDefaultRunnerModel(ctx context.Context, userID int, runner string, model string) error
}

type ChatSessionSettingsRepository interface {
	GetBySessionID(ctx context.Context, sessionID int64) (*ChatSessionSettings, error)

	Upsert(ctx context.Context, settings *ChatSessionSettings) error
}

type MessageRepository interface {
	Create(ctx context.Context, message *Message) error

	UpdateContent(ctx context.Context, id int64, content string) error

	GetBySessionId(ctx context.Context, sessionID int64, page, pageSize int32) ([]*Message, int32, error)
}

type FileRepository interface {
	Create(ctx context.Context, file *File) error

	UpdateStoragePath(ctx context.Context, id int64, storagePath string) error

	GetById(ctx context.Context, id int64) (*File, error)
}

type EditorHistoryRepository interface {
	Save(ctx context.Context, userID int, runner string, text string) error
}

type ResponseFormat struct {
	Type   string
	Schema *string
}

type Tool struct {
	Name           string
	Description    string
	ParametersJSON string
}

type GenerationParams struct {
	Temperature    *float32
	MaxTokens      *int32
	TopK           *int32
	TopP           *float32
	ResponseFormat *ResponseFormat
	Tools          []Tool
}

type LLMRepository interface {
	CheckConnection(ctx context.Context) (bool, error)

	GetModels(ctx context.Context) ([]string, error)

	SendMessage(
		ctx context.Context,
		sessionID int64,
		model string,
		messages []*Message,
		stopSequences []string,
		timeoutSeconds int32,
		genParams *GenerationParams,
	) (chan string, error)

	Embed(ctx context.Context, model string, text string) ([]float32, error)

	EmbedBatch(ctx context.Context, model string, texts []string) ([][]float32, error)
}
