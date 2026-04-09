package mcpclient

import (
	"context"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/magomedcoder/gen/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func waitToolsCacheGone(t *testing.T, c *ToolsListCache, key listCacheKey) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.RLock()
		_, ok := c.toolEntries[key]
		c.mu.RUnlock()
		if !ok {
			return
		}

		time.Sleep(15 * time.Millisecond)
	}

	t.Fatal("ожидалась инвалидация tools-кэша по notifications/tools/list_changed")
}

func waitResourcesCacheGone(t *testing.T, c *ToolsListCache, key listCacheKey) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.RLock()
		_, ok := c.resEntries[key]
		c.mu.RUnlock()
		if !ok {
			return
		}

		time.Sleep(15 * time.Millisecond)
	}

	t.Fatal("ожидалась инвалидация resources-кэша по notifications/resources/list_changed")
}

func waitPromptsCacheGone(t *testing.T, c *ToolsListCache, key listCacheKey) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.RLock()
		_, ok := c.promptsEntries[key]
		c.mu.RUnlock()
		if !ok {
			return
		}

		time.Sleep(15 * time.Millisecond)
	}

	t.Fatal("ожидалась инвалидация prompts-кэша по notifications/prompts/list_changed")
}

func TestListChangedNotificationsInvalidateToolsListCache(t *testing.T) {
	ctx := context.Background()
	cache := NewToolsListCache()
	t.Cleanup(func() { SetToolsListCacheForNotifications(nil) })

	dummy := &domain.MCPServer{
		ID:             701,
		Transport:      "stdio",
		Command:        "/bin/true",
		ArgsJSON:       `[]`,
		EnvJSON:        `{}`,
		HeadersJSON:    `{}`,
		TimeoutSeconds: 120,
	}
	fp := serverConfigFingerprint(dummy)
	key := listCacheKey{
		id: dummy.ID,
		fp: fp,
	}

	cache.mu.Lock()
	cache.toolEntries[key] = toolsCacheEntry{
		until: time.Now().Add(time.Hour),
	}
	cache.mu.Unlock()

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "lc-tools",
		Version: "1",
	}, nil)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "first",
		Description: "x",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})

	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ss.Close()
	})

	opts := buildMCPClientOptions(ctx, dummy, cache)
	cli := mcp.NewClient(&mcp.Implementation{
		Name:    "gen",
		Version: "1.0.0",
	}, opts)
	cs, err := cli.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	mcp.AddTool(srv, &mcp.Tool{
		Name: "second",
		InputSchema: &jsonschema.Schema{
			Type: "object",
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})

	waitToolsCacheGone(t, cache, key)
}

func TestListChangedNotificationsInvalidateResourcesCache(t *testing.T) {
	ctx := context.Background()
	cache := NewToolsListCache()
	t.Cleanup(func() { SetToolsListCacheForNotifications(nil) })

	dummy := &domain.MCPServer{
		ID:             702,
		Transport:      "stdio",
		Command:        "/bin/true",
		ArgsJSON:       `[]`,
		EnvJSON:        `{}`,
		HeadersJSON:    `{}`,
		TimeoutSeconds: 120,
	}
	fp := serverConfigFingerprint(dummy)
	key := listCacheKey{
		id: dummy.ID,
		fp: fp,
	}

	cache.mu.Lock()
	cache.resEntries[key] = resourcesCacheEntry{until: time.Now().Add(time.Hour)}
	cache.mu.Unlock()

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "lc-res",
		Version: "1",
	}, nil)
	srv.AddResource(&mcp.Resource{
		URI:  "file:///a",
		Name: "a",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{}, nil
	})

	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ss.Close()
	})

	opts := buildMCPClientOptions(ctx, dummy, cache)
	cli := mcp.NewClient(&mcp.Implementation{
		Name:    "gen",
		Version: "1.0.0",
	}, opts)
	cs, err := cli.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cs.Close()
	})

	srv.AddResource(&mcp.Resource{
		URI:  "file:///b",
		Name: "b",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{}, nil
	})

	waitResourcesCacheGone(t, cache, key)
}

func TestListChangedNotificationsInvalidatePromptsCache(t *testing.T) {
	ctx := context.Background()
	cache := NewToolsListCache()
	t.Cleanup(func() { SetToolsListCacheForNotifications(nil) })

	dummy := &domain.MCPServer{
		ID:             703,
		Transport:      "stdio",
		Command:        "/bin/true",
		ArgsJSON:       `[]`,
		EnvJSON:        `{}`,
		HeadersJSON:    `{}`,
		TimeoutSeconds: 120,
	}
	fp := serverConfigFingerprint(dummy)
	key := listCacheKey{
		id: dummy.ID,
		fp: fp,
	}

	cache.mu.Lock()
	cache.promptsEntries[key] = promptsCacheEntry{until: time.Now().Add(time.Hour)}
	cache.mu.Unlock()

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "lc-pr",
		Version: "1",
	}, nil)
	srv.AddPrompt(&mcp.Prompt{Name: "p1"}, func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{}, nil
	})

	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ss.Close()
	})

	opts := buildMCPClientOptions(ctx, dummy, cache)
	cli := mcp.NewClient(&mcp.Implementation{
		Name:    "gen",
		Version: "1.0.0",
	}, opts)
	cs, err := cli.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cs.Close()
	})

	srv.AddPrompt(&mcp.Prompt{Name: "p2"}, func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{}, nil
	})

	waitPromptsCacheGone(t, cache, key)
}
