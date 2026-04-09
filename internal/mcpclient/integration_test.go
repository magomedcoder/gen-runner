package mcpclient

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestIntegrationInMemoryListAndCall(t *testing.T) {
	ctx := context.Background()

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "stub",
		Version: "0.0.1",
	}, nil)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ping",
		Description: "test tool",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{
			Text: "pong",
		}}}, nil, nil
	})

	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	cli := mcp.NewClient(&mcp.Implementation{
		Name:    "gen",
		Version: "1.0.0",
	}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{},
	})
	session, err := cli.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = session.Close() })

	list, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	var names []string
	for _, tool := range list.Tools {
		if tool != nil && tool.Name != "" {
			names = append(names, tool.Name)
		}
	}

	if len(names) != 1 || names[0] != "ping" {
		t.Fatalf("tools: получен %v, требуется [ping]", names)
	}

	raw, _ := json.Marshal(map[string]any{})
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ping", Arguments: json.RawMessage(raw)})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	out := strings.TrimSpace(CallToolResultString(res))
	if out != "pong" {
		t.Fatalf("result: получил %q, требуется pong", out)
	}
}

func TestInputSchemaToParametersJSONRoundTrip(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"q": map[string]any{"type": "string"},
		},
	}

	j := inputSchemaToParametersJSON(schema)
	var m map[string]any
	if err := json.Unmarshal([]byte(j), &m); err != nil {
		t.Fatal(err)
	}

	if m["type"] != "object" {
		t.Fatalf("неожиданная схема json: %s", j)
	}
}

func TestValidateHTTPMCPURLLocalhostAllowed(t *testing.T) {
	old := httpHostPolicy
	defer func() { httpHostPolicy = old }()
	httpHostPolicy = func(host string) bool {
		return host == "localhost" || host == "127.0.0.1"
	}

	if err := validateHTTPMCPURL("sse", "http://localhost:8080/mcp"); err != nil {
		t.Fatal(err)
	}

	if err := validateHTTPMCPURL("sse", "http://test.test/mcp"); err == nil {
		t.Fatal("ожидаемый отказ")
	}
}
