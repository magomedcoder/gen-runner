package mcpclient

import (
	"context"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBuildServerProbeInMemoryMCP(t *testing.T) {
	ctx := context.Background()

	tool := &mcp.Tool{
		Name: "echo",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "probe-srv",
		Version: "9.8.7",
	}, &mcp.ServerOptions{
		Instructions: "test instructions",
	})

	s.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})

	s.AddPrompt(&mcp.Prompt{Name: "p1"}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{}, nil
	})

	s.AddResource(&mcp.Resource{URI: "file:///x", Name: "n"}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{}, nil
	})

	ct, st := mcp.NewInMemoryTransports()
	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()

	c := mcp.NewClient(&mcp.Implementation{Name: "gen", Version: "1.0.0"}, nil)
	cs, err := c.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	p, err := buildServerProbe(cs)
	if err != nil {
		t.Fatal(err)
	}

	if p.ServerName != "probe-srv" || p.ServerVersion != "9.8.7" {
		t.Fatalf("ServerInfo: got name=%q version=%q", p.ServerName, p.ServerVersion)
	}

	if p.Instructions != "test instructions" {
		t.Fatalf("Instructions: got %q", p.Instructions)
	}

	if p.ProtocolVersion == "" {
		t.Fatal("ожидалась непустая ProtocolVersion")
	}

	if !p.HasTools || !p.HasResources || !p.HasPrompts {
		t.Fatalf("capabilities: tools=%v resources=%v prompts=%v", p.HasTools, p.HasResources, p.HasPrompts)
	}
}
