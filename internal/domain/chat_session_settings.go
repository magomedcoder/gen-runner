package domain

type ChatSessionSettings struct {
	SessionID             int64
	SystemPrompt          string
	StopSequences         []string
	TimeoutSeconds        int32
	Temperature           *float32
	TopK                  *int32
	TopP                  *float32
	JSONMode              bool
	JSONSchema            string
	ToolsJSON             string
	Profile               string
	ModelReasoningEnabled bool
	WebSearchEnabled      bool
	WebSearchProvider     string
}
