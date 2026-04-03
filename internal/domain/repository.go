package domain

import (
	"context"
	"time"
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

	ListLatestMessagesForSession(ctx context.Context, sessionID int64, limit int32) ([]*Message, error)

	ListBySessionBeforeID(ctx context.Context, sessionID int64, beforeMessageID int64, limit int32) ([]*Message, int32, error)

	SessionHasOlderMessages(ctx context.Context, sessionID int64, olderThanMessageID int64) (bool, error)

	GetByID(ctx context.Context, id int64) (*Message, error)

	ListMessagesWithIDLessThan(ctx context.Context, sessionID int64, beforeMessageID int64) ([]*Message, error)

	ListMessagesUpToID(ctx context.Context, sessionID int64, upToMessageID int64) ([]*Message, error)

	SoftDeleteAfterID(ctx context.Context, sessionID int64, afterMessageID int64) error

	SoftDeleteRangeAfterID(ctx context.Context, sessionID int64, afterMessageID int64, upToMessageID int64) error

	ResetAssistantForRegenerate(ctx context.Context, sessionID int64, messageID int64) error

	MaxMessageIDInSession(ctx context.Context, sessionID int64) (int64, error)

	ListBySessionCreatedAtWindowIncludingDeleted(ctx context.Context, sessionID int64, fromInclusive, toExclusive time.Time) ([]*Message, error)
}

type MessageEditRepository interface {
	Create(ctx context.Context, edit *MessageEdit) error

	ListByMessageID(ctx context.Context, messageID int64, limit int32) ([]*MessageEdit, error)
}

type AssistantMessageRegenerationRepository interface {
	Create(ctx context.Context, regen *AssistantMessageRegeneration) error

	ListByMessageID(ctx context.Context, messageID int64, limit int32) ([]*AssistantMessageRegeneration, error)
}

type FileRepository interface {
	Create(ctx context.Context, file *File) error

	UpdateStoragePath(ctx context.Context, id int64, storagePath string) error

	GetById(ctx context.Context, id int64) (*File, error)

	ListByIds(ctx context.Context, ids []int64) ([]*File, error)

	SaveExtractedTextCache(ctx context.Context, fileID int64, contentSHA256Hex string, extractedText string) error

	CountSessionTTLArtifacts(ctx context.Context, sessionID int64, userID int) (count int32, totalSize int64, err error)
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

	SendMessageWithRunnerToolAction(
		ctx context.Context,
		sessionID int64,
		model string,
		messages []*Message,
		stopSequences []string,
		timeoutSeconds int32,
		genParams *GenerationParams,
	) (textChunks chan string, runnerToolJSON func() string, err error)

	Embed(ctx context.Context, model string, text string) ([]float32, error)

	EmbedBatch(ctx context.Context, model string, texts []string) ([][]float32, error)
}

type ChatRepos struct {
	Session         ChatSessionRepository
	Preference      ChatPreferenceRepository
	SessionSettings ChatSessionSettingsRepository
	Message         MessageRepository
	MessageEdit     MessageEditRepository
	AssistantRegen  AssistantMessageRegenerationRepository
	File            FileRepository
}

type ChatTransactionRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, r ChatRepos) error) error
}

type AuthRepos struct {
	User  UserRepository
	Token TokenRepository
}

type AuthTransactionRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, r AuthRepos) error) error
}
