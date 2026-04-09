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

	m := MCPCountersMap()
	if m["list_tools_fail"] < 1 || m["list_tools_ok"] < 1 {
		t.Fatalf("list_tools counters: %v", m)
	}

	if m["call_tool_fail"] < 1 || m["call_tool_mcp_error"] < 1 || m["call_tool_ok"] < 1 {
		t.Fatalf("call_tool counters: %v", m)
	}
}
