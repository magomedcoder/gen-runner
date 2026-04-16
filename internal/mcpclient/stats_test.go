package mcpclient

import (
	"errors"
	"testing"
)

func TestMCPCountersMapIncrements(t *testing.T) {
	recordListTools(errors.New("x"))
	recordListTools(nil)
	recordCallToolTransportErr()
	recordCallToolMCPError()
	recordCallToolOK()
	recordCallToolRetry()
	recordListCacheHit()
	recordListCacheMiss()
	recordPooledSessionRetry()
	recordCallToolServer(99, "ok")
	recordCallToolServer(99, "transport_err")

	m := MCPCountersMap()
	if m["list_tools_fail"] < 1 || m["list_tools_ok"] < 1 {
		t.Fatalf("list_tools counters: %v", m)
	}

	if m["call_tool_fail"] < 1 || m["call_tool_mcp_error"] < 1 || m["call_tool_ok"] < 1 {
		t.Fatalf("call_tool counters: %v", m)
	}

	if m["call_tool_retry"] < 1 {
		t.Fatalf("call_tool_retry counter: %v", m)
	}

	if m["list_cache_hit"] < 1 || m["list_cache_miss"] < 1 {
		t.Fatalf("list_cache counters: %v", m)
	}

	if m["pooled_session_retry"] < 1 {
		t.Fatalf("pooled_session_retry counter: %v", m)
	}

	if m["call_tool_server_99_ok"] < 1 || m["call_tool_server_99_transport_err"] < 1 {
		t.Fatalf("per-server call_tool counters: %v", m)
	}
}
