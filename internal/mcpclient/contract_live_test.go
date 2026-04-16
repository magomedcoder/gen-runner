package mcpclient

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
)

func TestActiveMCPServersContractLive(t *testing.T) {
	raw := strings.TrimSpace(os.Getenv("MCP_ACTIVE_CONTRACTS_JSON"))
	if raw == "" {
		t.Skip("set MCP_ACTIVE_CONTRACTS_JSON to run live MCP contract tests")
	}

	var servers []*domain.MCPServer
	if err := json.Unmarshal([]byte(raw), &servers); err != nil {
		t.Fatalf("MCP_ACTIVE_CONTRACTS_JSON parse: %v", err)
	}

	var active []*domain.MCPServer
	for _, s := range servers {
		if s != nil && s.Enabled && s.ID > 0 {
			active = append(active, s)
		}
	}
	if len(active) == 0 {
		t.Fatalf("no enabled servers in MCP_ACTIVE_CONTRACTS_JSON")
	}

	cache := NewToolsListCache()
	for _, srv := range active {
		srv := srv
		t.Run(srv.Name, func(t *testing.T) {
			timeout := 45 * time.Second
			if srv.TimeoutSeconds > 0 {
				timeout = time.Duration(srv.TimeoutSeconds) * time.Second
				if timeout > 180*time.Second {
					timeout = 180 * time.Second
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			probe, err := ProbeServer(ctx, srv, cache)
			if err != nil {
				t.Fatalf("probe failed: %v", err)
			}

			if strings.TrimSpace(probe.ProtocolVersion) == "" {
				t.Fatalf("empty protocol version")
			}

			_, err = cache.ListToolsCached(ctx, srv, DefaultToolsListCacheTTL)
			if err != nil {
				t.Fatalf("list tools failed: %v", err)
			}
		})
	}
}
