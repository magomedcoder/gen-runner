package mcpclient

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type mcpRPCStats struct {
	listToolsOK, listToolsFail         atomic.Uint64
	listResourcesOK, listResourcesFail atomic.Uint64
	listPromptsOK, listPromptsFail     atomic.Uint64
	readResourceOK, readResourceFail   atomic.Uint64
	getPromptOK, getPromptFail         atomic.Uint64
	callToolOK, callToolFail           atomic.Uint64
	callToolMCPError                   atomic.Uint64
	callToolRetry                      atomic.Uint64
	probeOK, probeFail                 atomic.Uint64
	listCacheHit, listCacheMiss        atomic.Uint64
	pooledSessionRetry                 atomic.Uint64
}

var rpcStats mcpRPCStats

type callToolServerStat struct {
	ok           atomic.Uint64
	transportErr atomic.Uint64
	mcpError     atomic.Uint64
}

var callToolByServer sync.Map

type CallToolServerCounters struct {
	OK           uint64
	TransportErr uint64
	MCPError     uint64
}

func recordCallToolServer(serverID int64, outcome string) {
	if serverID <= 0 {
		return
	}

	maxN := maxTrackedCallStatServerIDs.Load()
	if maxN > 0 {
		if _, ok := callToolByServer.Load(serverID); !ok {
			if callToolStatDistinctCount.Load() >= uint64(maxN) {
				return
			}
		}
	}

	v, loaded := callToolByServer.LoadOrStore(serverID, &callToolServerStat{})
	if !loaded {
		callToolStatDistinctCount.Add(1)
	}

	st := v.(*callToolServerStat)
	switch outcome {
	case "ok":
		st.ok.Add(1)
	case "transport_err":
		st.transportErr.Add(1)
	case "mcp_error":
		st.mcpError.Add(1)
	}
}

func callToolServerCountersSnapshot() map[int64]CallToolServerCounters {
	out := map[int64]CallToolServerCounters{}
	callToolByServer.Range(func(k, v any) bool {
		id, ok := k.(int64)
		if !ok {
			return true
		}
		st, ok := v.(*callToolServerStat)
		if !ok || st == nil {
			return true
		}
		out[id] = CallToolServerCounters{
			OK:           st.ok.Load(),
			TransportErr: st.transportErr.Load(),
			MCPError:     st.mcpError.Load(),
		}
		return true
	})
	return out
}

func recordListTools(err error) {
	if err != nil {
		rpcStats.listToolsFail.Add(1)
	} else {
		rpcStats.listToolsOK.Add(1)
	}
}

func recordListResources(err error) {
	if err != nil {
		rpcStats.listResourcesFail.Add(1)
	} else {
		rpcStats.listResourcesOK.Add(1)
	}
}

func recordListPrompts(err error) {
	if err != nil {
		rpcStats.listPromptsFail.Add(1)
	} else {
		rpcStats.listPromptsOK.Add(1)
	}
}

func recordReadResource(err error) {
	if err != nil {
		rpcStats.readResourceFail.Add(1)
	} else {
		rpcStats.readResourceOK.Add(1)
	}
}

func recordGetPrompt(err error) {
	if err != nil {
		rpcStats.getPromptFail.Add(1)
	} else {
		rpcStats.getPromptOK.Add(1)
	}
}

func recordCallToolTransportErr() {
	rpcStats.callToolFail.Add(1)
}

func recordCallToolMCPError() {
	rpcStats.callToolMCPError.Add(1)
}

func recordCallToolOK() {
	rpcStats.callToolOK.Add(1)
}

func recordCallToolRetry() {
	rpcStats.callToolRetry.Add(1)
}

func recordProbe(err error) {
	if err != nil {
		rpcStats.probeFail.Add(1)
	} else {
		rpcStats.probeOK.Add(1)
	}
}

func recordListCacheHit() {
	rpcStats.listCacheHit.Add(1)
}

func recordListCacheMiss() {
	rpcStats.listCacheMiss.Add(1)
}

func recordPooledSessionRetry() {
	rpcStats.pooledSessionRetry.Add(1)
}

func MCPCountersMap() map[string]uint64 {
	out := map[string]uint64{
		"list_tools_ok":        rpcStats.listToolsOK.Load(),
		"list_tools_fail":      rpcStats.listToolsFail.Load(),
		"list_resources_ok":    rpcStats.listResourcesOK.Load(),
		"list_resources_fail":  rpcStats.listResourcesFail.Load(),
		"list_prompts_ok":      rpcStats.listPromptsOK.Load(),
		"list_prompts_fail":    rpcStats.listPromptsFail.Load(),
		"read_resource_ok":     rpcStats.readResourceOK.Load(),
		"read_resource_fail":   rpcStats.readResourceFail.Load(),
		"get_prompt_ok":        rpcStats.getPromptOK.Load(),
		"get_prompt_fail":      rpcStats.getPromptFail.Load(),
		"call_tool_ok":         rpcStats.callToolOK.Load(),
		"call_tool_fail":       rpcStats.callToolFail.Load(),
		"call_tool_mcp_error":  rpcStats.callToolMCPError.Load(),
		"call_tool_retry":      rpcStats.callToolRetry.Load(),
		"probe_ok":             rpcStats.probeOK.Load(),
		"probe_fail":           rpcStats.probeFail.Load(),
		"list_cache_hit":       rpcStats.listCacheHit.Load(),
		"list_cache_miss":      rpcStats.listCacheMiss.Load(),
		"pooled_session_retry": rpcStats.pooledSessionRetry.Load(),
	}

	callToolByServer.Range(func(k, v any) bool {
		id, ok := k.(int64)
		if !ok {
			return true
		}

		st, ok := v.(*callToolServerStat)
		if !ok || st == nil {
			return true
		}

		prefix := fmt.Sprintf("call_tool_server_%d_", id)
		out[prefix+"ok"] = st.ok.Load()
		out[prefix+"transport_err"] = st.transportErr.Load()
		out[prefix+"mcp_error"] = st.mcpError.Load()
		return true
	})

	return out
}
