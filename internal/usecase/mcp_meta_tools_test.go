package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
)

type fakeChatSessionRepoForMCP struct {
	session *domain.ChatSession
}

func (f fakeChatSessionRepoForMCP) Create(context.Context, *domain.ChatSession) error {
	return nil
}

func (f fakeChatSessionRepoForMCP) GetById(context.Context, int64) (*domain.ChatSession, error) {
	if f.session != nil {
		return f.session, nil
	}

	return &domain.ChatSession{
		Id:     1,
		UserId: 42,
	}, nil
}

func (f fakeChatSessionRepoForMCP) GetByUserId(context.Context, int, int32, int32) ([]*domain.ChatSession, int32, error) {
	return nil, 0, nil
}

func (f fakeChatSessionRepoForMCP) Update(context.Context, *domain.ChatSession) error {
	return nil
}

func (f fakeChatSessionRepoForMCP) Delete(context.Context, int64) error {
	return nil
}

type fakeSessionSettingsRepoForMCP struct {
	st *domain.ChatSessionSettings
}

func (f fakeSessionSettingsRepoForMCP) GetBySessionID(context.Context, int64) (*domain.ChatSessionSettings, error) {
	return f.st, nil
}

func (fakeSessionSettingsRepoForMCP) Upsert(context.Context, *domain.ChatSessionSettings) error {
	return nil
}

type fakeMCPServerRepoForMCP struct {
	accessible func(ctx context.Context, id int64, userID int) (*domain.MCPServer, error)
}

func (fakeMCPServerRepoForMCP) ListGlobal(context.Context) ([]*domain.MCPServer, error) {
	return nil, nil
}

func (fakeMCPServerRepoForMCP) ListForUser(context.Context, int) ([]*domain.MCPServer, error) {
	return nil, nil
}

func (fakeMCPServerRepoForMCP) ListActive(context.Context) ([]*domain.MCPServer, error) {
	return nil, nil
}

func (fakeMCPServerRepoForMCP) GetByID(context.Context, int64) (*domain.MCPServer, error) {
	return nil, nil
}

func (f fakeMCPServerRepoForMCP) GetByIDAccessible(ctx context.Context, id int64, userID int) (*domain.MCPServer, error) {
	if f.accessible != nil {
		return f.accessible(ctx, id, userID)
	}

	return nil, nil
}

func (fakeMCPServerRepoForMCP) GetGlobalByID(context.Context, int64) (*domain.MCPServer, error) {
	return nil, nil
}

func (fakeMCPServerRepoForMCP) Create(context.Context, *domain.MCPServer) (*domain.MCPServer, error) {
	return nil, nil
}

func (fakeMCPServerRepoForMCP) UpdateGlobal(context.Context, *domain.MCPServer) error {
	return nil
}

func (fakeMCPServerRepoForMCP) UpdateOwned(context.Context, *domain.MCPServer, int) error {
	return nil
}

func (fakeMCPServerRepoForMCP) DeleteGlobal(context.Context, int64) error {
	return nil
}

func (fakeMCPServerRepoForMCP) DeleteOwned(context.Context, int64, int) error {
	return nil
}

func (fakeMCPServerRepoForMCP) CountOwnedByUser(context.Context, int) (int64, error) {
	return 0, nil
}

func TestGenMcpMetaToolsMCPServerRepoNil(t *testing.T) {
	ctx := context.Background()
	c := &ChatUseCase{
		sessionRepo: fakeChatSessionRepoForMCP{},
		sessionSettingsRepo: fakeSessionSettingsRepoForMCP{
			st: &domain.ChatSessionSettings{
				MCPEnabled:   true,
				MCPServerIDs: []int64{5},
			},
		},
		mcpServerRepo: nil,
	}
	_, err := c.toolGenMcpListResources(ctx, 1, json.RawMessage(`{"server_id":5}`))
	if err == nil || !strings.Contains(err.Error(), "MCP недоступен") {
		t.Fatalf("got %v", err)
	}
}

func TestGenMcpMetaToolsMCPDisabled(t *testing.T) {
	ctx := context.Background()
	c := &ChatUseCase{
		sessionRepo: fakeChatSessionRepoForMCP{},
		sessionSettingsRepo: fakeSessionSettingsRepoForMCP{
			st: &domain.ChatSessionSettings{
				MCPEnabled:   false,
				MCPServerIDs: []int64{5},
			},
		},
		mcpServerRepo: fakeMCPServerRepoForMCP{},
	}
	_, err := c.toolGenMcpListResources(ctx, 1, json.RawMessage(`{"server_id":5}`))
	if err == nil || !strings.Contains(err.Error(), "MCP отключён") {
		t.Fatalf("got %v", err)
	}
}

func TestGenMcpMetaToolsServerNotSelectedForSession(t *testing.T) {
	ctx := context.Background()
	c := &ChatUseCase{
		sessionRepo: fakeChatSessionRepoForMCP{},
		sessionSettingsRepo: fakeSessionSettingsRepoForMCP{
			st: &domain.ChatSessionSettings{
				MCPEnabled:   true,
				MCPServerIDs: []int64{1, 2},
			},
		},
		mcpServerRepo: fakeMCPServerRepoForMCP{},
	}
	_, err := c.toolGenMcpListPrompts(ctx, 1, json.RawMessage(`{"server_id":99}`))
	if err == nil || !strings.Contains(err.Error(), "не выбран") {
		t.Fatalf("got %v", err)
	}
}

func TestGenMcpMetaToolsServerNilOrDisabled(t *testing.T) {
	ctx := context.Background()
	c := &ChatUseCase{
		sessionRepo: fakeChatSessionRepoForMCP{},
		sessionSettingsRepo: fakeSessionSettingsRepoForMCP{
			st: &domain.ChatSessionSettings{
				MCPEnabled:   true,
				MCPServerIDs: []int64{5},
			},
		},
		mcpServerRepo: fakeMCPServerRepoForMCP{
			accessible: func(context.Context, int64, int) (*domain.MCPServer, error) {
				return nil, nil
			},
		},
	}
	_, err := c.toolGenMcpReadResource(ctx, 1, json.RawMessage(`{"server_id":5,"uri":"x://a"}`))
	if err == nil || !strings.Contains(err.Error(), "недоступен") {
		t.Fatalf("got %v", err)
	}
}

func TestGenMcpMetaToolsInvalidServerID(t *testing.T) {
	ctx := context.Background()
	c := &ChatUseCase{
		sessionRepo: fakeChatSessionRepoForMCP{},
		sessionSettingsRepo: fakeSessionSettingsRepoForMCP{
			st: &domain.ChatSessionSettings{
				MCPEnabled:   true,
				MCPServerIDs: []int64{5},
			},
		},
		mcpServerRepo: fakeMCPServerRepoForMCP{},
	}
	_, err := c.toolGenMcpListResources(ctx, 1, json.RawMessage(`{"server_id":0}`))
	if err == nil || !strings.Contains(err.Error(), "некорректный") {
		t.Fatalf("got %v", err)
	}
}

func TestGenMcpGetPromptMissingName(t *testing.T) {
	ctx := context.Background()
	c := &ChatUseCase{
		sessionRepo: fakeChatSessionRepoForMCP{},
		sessionSettingsRepo: fakeSessionSettingsRepoForMCP{
			st: &domain.ChatSessionSettings{
				MCPEnabled:   true,
				MCPServerIDs: []int64{5},
			},
		},
		mcpServerRepo: fakeMCPServerRepoForMCP{},
	}
	_, err := c.toolGenMcpGetPrompt(ctx, 1, json.RawMessage(`{"server_id":5,"name":"  "}`))
	if err == nil || !strings.Contains(err.Error(), "нужны server_id и name") {
		t.Fatalf("got %v", err)
	}
}

func TestMaybeInjectMCPBuiltinMetaToolsAddsFourTools(t *testing.T) {
	c := &ChatUseCase{
		mcpServerRepo: fakeMCPServerRepoForMCP{},
	}
	gp := &domain.GenerationParams{
		Tools: []domain.Tool{},
	}
	st := &domain.ChatSessionSettings{
		MCPEnabled:   true,
		MCPServerIDs: []int64{7},
	}

	c.maybeInjectMCPBuiltinMetaTools(gp, st)
	if len(gp.Tools) != 4 {
		t.Fatalf("ожидалось 4 инструмента, получено %d", len(gp.Tools))
	}

	got := make(map[string]struct{}, len(gp.Tools))
	for _, tool := range gp.Tools {
		got[normalizeToolName(tool.Name)] = struct{}{}
	}

	for _, want := range []string{
		"gen_mcp_list_resources",
		"gen_mcp_read_resource",
		"gen_mcp_list_prompts",
		"gen_mcp_get_prompt",
	} {
		if _, ok := got[want]; !ok {
			t.Fatalf("нет инструмента %q среди %v", want, gp.Tools)
		}
	}
}

func TestMaybeInjectMCPBuiltinMetaToolsNoInjectWhenDisabledOrEmpty(t *testing.T) {
	c := &ChatUseCase{
		mcpServerRepo: fakeMCPServerRepoForMCP{},
	}
	base := []domain.Tool{
		{
			Name: "only_tool",
		},
	}

	gp := &domain.GenerationParams{Tools: append([]domain.Tool(nil), base...)}
	c.maybeInjectMCPBuiltinMetaTools(gp, &domain.ChatSessionSettings{
		MCPEnabled:   false,
		MCPServerIDs: []int64{1},
	})
	if len(gp.Tools) != 1 {
		t.Fatalf("при отключённом MCP: получено %d инструментов", len(gp.Tools))
	}

	gp2 := &domain.GenerationParams{
		Tools: append([]domain.Tool(nil), base...),
	}
	c.maybeInjectMCPBuiltinMetaTools(gp2, &domain.ChatSessionSettings{
		MCPEnabled:   true,
		MCPServerIDs: nil,
	})
	if len(gp2.Tools) != 1 {
		t.Fatalf("пустые id серверов: получено %d инструментов", len(gp2.Tools))
	}

	gp3 := &domain.GenerationParams{Tools: append([]domain.Tool(nil), base...)}
	c3 := &ChatUseCase{mcpServerRepo: nil}
	c3.maybeInjectMCPBuiltinMetaTools(gp3, &domain.ChatSessionSettings{
		MCPEnabled:   true,
		MCPServerIDs: []int64{1},
	})
	if len(gp3.Tools) != 1 {
		t.Fatalf("nil-репозиторий: получено %d инструментов", len(gp3.Tools))
	}
}

func TestMaybeInjectMCPBuiltinMetaToolsSkipsNormalizedDuplicate(t *testing.T) {
	c := &ChatUseCase{
		mcpServerRepo: fakeMCPServerRepoForMCP{},
	}
	gp := &domain.GenerationParams{
		Tools: []domain.Tool{
			{
				Name: "GEN-MCP-LIST-RESOURCES",
			},
		},
	}
	st := &domain.ChatSessionSettings{
		MCPEnabled:   true,
		MCPServerIDs: []int64{1},
	}
	c.maybeInjectMCPBuiltinMetaTools(gp, st)
	if len(gp.Tools) != 4 {
		t.Fatalf("ожидалось 1 уже заданный + 3 новых = 4, получено %d", len(gp.Tools))
	}
}

func TestAppendMCPLLMContext(t *testing.T) {
	ctx := context.Background()
	c := &ChatUseCase{
		mcpServerRepo: fakeMCPServerRepoForMCP{
			accessible: func(_ context.Context, id int64, _ int) (*domain.MCPServer, error) {
				if id == 9 {
					return &domain.MCPServer{ID: 9, Name: "Demo", Enabled: true}, nil
				}

				return nil, nil
			},
		},
	}

	msg := domain.NewMessage(1, "base", domain.MessageRoleSystem)
	st := &domain.ChatSessionSettings{MCPEnabled: true, MCPServerIDs: []int64{9}}
	c.appendMCPLLMContext(ctx, msg, st, 1)
	if !strings.Contains(msg.Content, "base") {
		t.Fatal("потерян исходный текст системного сообщения")
	}

	if !strings.Contains(msg.Content, "id=9 · Demo") {
		t.Fatalf("ожидалось имя сервера: %q", msg.Content)
	}

	if !strings.Contains(msg.Content, "gen_mcp_list_resources") {
		t.Fatalf("ожидались примеры gen_mcp_*: %q", msg.Content)
	}

	msg2 := domain.NewMessage(1, "x", domain.MessageRoleSystem)
	c.appendMCPLLMContext(ctx, msg2, &domain.ChatSessionSettings{MCPEnabled: false, MCPServerIDs: []int64{9}}, 1)
	if msg2.Content != "x" {
		t.Fatalf("при выключенном MCP не должно быть дополнения: %q", msg2.Content)
	}
}
