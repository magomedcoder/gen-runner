package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
)

type scriptedLLMRound struct {
	text       string
	toolAction string
}

type scriptedLLMRepo struct {
	mu     sync.Mutex
	rounds []scriptedLLMRound
	next   int
}

func (s *scriptedLLMRepo) CheckConnection(context.Context) (bool, error) {
	return true, nil
}

func (s *scriptedLLMRepo) GetModels(context.Context) ([]string, error) {
	return []string{"x"}, nil
}

func (s *scriptedLLMRepo) SendMessage(context.Context, int64, string, []*domain.Message, []string, int32, *domain.GenerationParams) (chan domain.LLMStreamChunk, error) {
	return nil, fmt.Errorf("not used in tests")
}

func (s *scriptedLLMRepo) SendMessageWithRunnerToolAction(context.Context, int64, string, []*domain.Message, []string, int32, *domain.GenerationParams) (chan domain.LLMStreamChunk, func() string, error) {
	return nil, nil, fmt.Errorf("not used in tests")
}

func (s *scriptedLLMRepo) SendMessageOnRunner(context.Context, string, int64, string, []*domain.Message, []string, int32, *domain.GenerationParams) (chan domain.LLMStreamChunk, error) {
	return nil, fmt.Errorf("not used in tests")
}

func (s *scriptedLLMRepo) SendMessageWithRunnerToolActionOnRunner(_ context.Context, _ string, _ int64, _ string, _ []*domain.Message, _ []string, _ int32, _ *domain.GenerationParams) (chan domain.LLMStreamChunk, func() string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next >= len(s.rounds) {
		return nil, nil, fmt.Errorf("script exhausted")
	}

	r := s.rounds[s.next]
	s.next++
	ch := make(chan domain.LLMStreamChunk, 1)
	ch <- domain.LLMStreamChunk{
		Content: r.text,
	}

	close(ch)

	return ch, func() string { return r.toolAction }, nil
}

func (s *scriptedLLMRepo) Embed(context.Context, string, string) ([]float32, error) {
	return nil, fmt.Errorf("not used in tests")
}

func (s *scriptedLLMRepo) EmbedBatch(context.Context, string, []string) ([][]float32, error) {
	return nil, fmt.Errorf("not used in tests")
}

type memoryMessageRepo struct {
	mu       sync.Mutex
	nextID   int64
	messages []*domain.Message
}

func (m *memoryMessageRepo) Create(_ context.Context, message *domain.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nextID == 0 {
		m.nextID = 1
	}

	if message.Id == 0 {
		message.Id = m.nextID
		m.nextID++
	}

	cloned := *message
	m.messages = append(m.messages, &cloned)

	return nil
}
func (m *memoryMessageRepo) UpdateContent(context.Context, int64, string) error {
	return nil
}

func (m *memoryMessageRepo) GetBySessionId(context.Context, int64, int32, int32) ([]*domain.Message, int32, error) {
	return nil, 0, nil
}

func (m *memoryMessageRepo) ListLatestMessagesForSession(context.Context, int64, int32) ([]*domain.Message, error) {
	return nil, nil
}

func (m *memoryMessageRepo) ListBySessionBeforeID(context.Context, int64, int64, int32) ([]*domain.Message, int32, error) {
	return nil, 0, nil
}

func (m *memoryMessageRepo) SessionHasOlderMessages(context.Context, int64, int64) (bool, error) {
	return false, nil
}

func (m *memoryMessageRepo) GetByID(context.Context, int64) (*domain.Message, error) {
	return nil, nil
}

func (m *memoryMessageRepo) ListMessagesWithIDLessThan(context.Context, int64, int64) ([]*domain.Message, error) {
	return nil, nil
}

func (m *memoryMessageRepo) ListMessagesUpToID(context.Context, int64, int64) ([]*domain.Message, error) {
	return nil, nil
}
func (m *memoryMessageRepo) SoftDeleteAfterID(context.Context, int64, int64) error {
	return nil
}

func (m *memoryMessageRepo) SoftDeleteRangeAfterID(context.Context, int64, int64, int64) error {
	return nil
}

func (m *memoryMessageRepo) ResetAssistantForRegenerate(context.Context, int64, int64) error {
	return nil
}

func (m *memoryMessageRepo) MaxMessageIDInSession(context.Context, int64) (int64, error) {
	return 0, nil
}

func (m *memoryMessageRepo) ListBySessionCreatedAtWindowIncludingDeleted(context.Context, int64, time.Time, time.Time) ([]*domain.Message, error) {
	return nil, nil
}

type fakeChatTx struct {
	repos domain.ChatRepos
}

func (f fakeChatTx) WithinTx(ctx context.Context, fn func(context.Context, domain.ChatRepos) error) error {
	return fn(ctx, f.repos)
}

func markdownPatchActionJSON(base, patch string) string {
	row := []map[string]any{
		{
			"tool_name": "apply_markdown_patch",
			"parameters": map[string]string{
				"base_text":  base,
				"patch_json": patch,
			},
		},
	}
	b, _ := json.Marshal(row)
	return string(b)
}

func TestRunChatToolLoopSequentialCallsAndFinalAnswer(t *testing.T) {
	msgRepo := &memoryMessageRepo{}
	llm := &scriptedLLMRepo{
		rounds: []scriptedLLMRound{
			{
				text:       "Сначала правлю текст A->AB",
				toolAction: markdownPatchActionJSON("A", `{"ops":[{"op":"append","text":"B"}]}`),
			},
			{
				text:       "Теперь правлю AB->ABC",
				toolAction: markdownPatchActionJSON("AB", `{"ops":[{"op":"append","text":"C"}]}`),
			},
			{
				text:       "Итог: ABC",
				toolAction: "",
			},
		},
	}
	c := &ChatUseCase{
		llmRepo:     llm,
		messageRepo: msgRepo,
		chatTx: fakeChatTx{
			repos: domain.ChatRepos{
				Message: msgRepo,
			},
		},
	}

	out := make(chan ChatStreamChunk, 64)
	go c.runChatToolLoop(
		context.Background(),
		1,
		100,
		"runner",
		"model",
		[]*domain.Message{domain.NewMessage(100, "system", domain.MessageRoleSystem)},
		nil,
		60,
		&domain.GenerationParams{
			Tools: []domain.Tool{{Name: "apply_markdown_patch", ParametersJSON: `{}`}},
		},
		false,
		nil,
		out,
	)

	var chunks []ChatStreamChunk
	for ch := range out {
		chunks = append(chunks, ch)
	}

	if len(chunks) == 0 {
		t.Fatal("expected non-empty stream")
	}

	var statuses int
	for _, ch := range chunks {
		if ch.Kind == StreamChunkKindToolStatus {
			statuses++
		}
	}
	if statuses != 2 {
		t.Fatalf("expected 2 tool statuses, got=%d", statuses)
	}

	msgRepo.mu.Lock()
	defer msgRepo.mu.Unlock()
	if len(msgRepo.messages) != 5 {
		t.Fatalf("expected 5 persisted messages (2 rounds + final), got=%d", len(msgRepo.messages))
	}

	if msgRepo.messages[0].Role != domain.MessageRoleAssistant || msgRepo.messages[0].ToolCallsJSON == "" {
		t.Fatalf("first persisted message must be assistant with tool_calls_json: %+v", msgRepo.messages[0])
	}

	if msgRepo.messages[1].Role != domain.MessageRoleTool || strings.TrimSpace(msgRepo.messages[1].ToolCallID) != "call_1" {
		t.Fatalf("first tool message mismatch: %+v", msgRepo.messages[1])
	}

	if msgRepo.messages[3].Role != domain.MessageRoleTool || strings.TrimSpace(msgRepo.messages[3].ToolCallID) != "call_1" {
		t.Fatalf("second tool message mismatch: %+v", msgRepo.messages[3])
	}

	if msgRepo.messages[4].Role != domain.MessageRoleAssistant || strings.TrimSpace(msgRepo.messages[4].Content) != "Итог: ABC" {
		t.Fatalf("final assistant message mismatch: %+v", msgRepo.messages[4])
	}
}

func TestRunChatToolLoopUndeclaredToolStopsWithError(t *testing.T) {
	msgRepo := &memoryMessageRepo{}
	llm := &scriptedLLMRepo{
		rounds: []scriptedLLMRound{
			{
				text:       "Пробую неразрешённый вызов",
				toolAction: `[{"tool_name":"unknown_tool","parameters":{}}]`,
			},
		},
	}
	c := &ChatUseCase{
		llmRepo:     llm,
		messageRepo: msgRepo,
		chatTx: fakeChatTx{
			repos: domain.ChatRepos{
				Message: msgRepo,
			},
		},
	}

	out := make(chan ChatStreamChunk, 8)
	go c.runChatToolLoop(
		context.Background(),
		1,
		200,
		"runner",
		"model",
		[]*domain.Message{domain.NewMessage(200, "system", domain.MessageRoleSystem)},
		nil,
		60,
		&domain.GenerationParams{
			Tools: []domain.Tool{
				{
					Name:           "apply_markdown_patch",
					ParametersJSON: `{}`,
				},
			},
		},
		false,
		nil,
		out,
	)

	var gotErr string
	for ch := range out {
		if ch.Kind == StreamChunkKindText && strings.Contains(ch.Text, "не объявлен") {
			gotErr = ch.Text
		}
	}
	if gotErr == "" {
		t.Fatal("expected undeclared-tool error in stream")
	}

	msgRepo.mu.Lock()
	defer msgRepo.mu.Unlock()
	if len(msgRepo.messages) != 0 {
		t.Fatalf("no messages should be persisted on undeclared tool, got=%d", len(msgRepo.messages))
	}
}
