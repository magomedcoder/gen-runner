package usecase

import (
	"context"
	"strings"
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
)

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

func TestAppendMCPLLMContextContainsMCPToolHintsWithoutGenMetaTools(t *testing.T) {
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

	if !strings.Contains(msg.Content, "mcp_<id>_h<hex>") {
		t.Fatalf("ожидалась подсказка про alias MCP tools: %q", msg.Content)
	}

	if strings.Contains(msg.Content, "gen_mcp_") {
		t.Fatalf("в системной подсказке не должно быть gen_mcp_*: %q", msg.Content)
	}
}

func TestAppendMCPLLMContextDisabledNoop(t *testing.T) {
	ctx := context.Background()
	c := &ChatUseCase{
		mcpServerRepo: fakeMCPServerRepoForMCP{},
	}

	msg := domain.NewMessage(1, "x", domain.MessageRoleSystem)
	c.appendMCPLLMContext(ctx, msg, &domain.ChatSessionSettings{MCPEnabled: false, MCPServerIDs: []int64{9}}, 1)
	if msg.Content != "x" {
		t.Fatalf("при выключенном MCP не должно быть дополнения: %q", msg.Content)
	}
}
