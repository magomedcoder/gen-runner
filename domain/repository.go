package domain

import "context"

type ResponseFormat struct {
	Type   string
	Schema *string
}

type Tool struct {
	Name           string
	Description    string
	ParametersJSON string
}

type ToolCall struct {
	Id        string
	Name      string
	Arguments string
}

type GenerationParams struct {
	Temperature    *float32
	MaxTokens      *int32
	TopK           *int32
	TopP           *float32
	ResponseFormat *ResponseFormat
	Tools          []Tool
}

type LLMProvider interface {
	CheckConnection(ctx context.Context) (bool, error)

	GetModels(ctx context.Context) ([]string, error)

	SendMessage(ctx context.Context, sessionID int64, model string, messages []*AIChatMessage, stopSequences []string, timeoutSeconds int32, genParams *GenerationParams) (chan string, error)
}
