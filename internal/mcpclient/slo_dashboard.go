package mcpclient

import (
	"encoding/json"
	"sort"
	"sync"
	"time"
)

const (
	defaultSLOTargetRatio = 0.99
)

type ActiveServerDescriptor struct {
	ID   int64
	Name string
}

type ActiveServerCatalogProvider func() []ActiveServerDescriptor

var (
	activeServerCatalogMu       sync.RWMutex
	activeServerCatalogProvider ActiveServerCatalogProvider
)

func SetActiveServerCatalogProvider(p ActiveServerCatalogProvider) {
	activeServerCatalogMu.Lock()
	defer activeServerCatalogMu.Unlock()
	activeServerCatalogProvider = p
}

func activeServerCatalog() []ActiveServerDescriptor {
	activeServerCatalogMu.RLock()
	p := activeServerCatalogProvider
	activeServerCatalogMu.RUnlock()
	if p == nil {
		return nil
	}

	list := p()
	if len(list) == 0 {
		return nil
	}

	out := make([]ActiveServerDescriptor, 0, len(list))
	for _, s := range list {
		if s.ID <= 0 {
			continue
		}
		out = append(out, ActiveServerDescriptor{
			ID:   s.ID,
			Name: s.Name,
		})
	}

	return out
}

type MCPServerSLORow struct {
	ServerID        int64   `json:"server_id"`
	ServerName      string  `json:"server_name,omitempty"`
	CallTotal       uint64  `json:"call_total"`
	SuccessTotal    uint64  `json:"success_total"`
	TransportErrors uint64  `json:"transport_errors"`
	MCPErrors       uint64  `json:"mcp_errors"`
	SuccessRatio    float64 `json:"success_ratio"`
	ErrorRatio      float64 `json:"error_ratio"`
	TargetRatio     float64 `json:"target_ratio"`
	TargetMet       bool    `json:"target_met"`
	UpdatedAt       string  `json:"updated_at"`
}

func MCPServerSLODashboard() []MCPServerSLORow {
	counters := callToolServerCountersSnapshot()
	catalog := activeServerCatalog()
	now := time.Now().UTC().Format(time.RFC3339)
	target := defaultSLOTargetRatio

	byID := map[int64]MCPServerSLORow{}
	for id, c := range counters {
		total := c.OK + c.TransportErr + c.MCPError
		success := 1.0
		errRatio := 0.0
		if total > 0 {
			success = float64(c.OK) / float64(total)
			errRatio = 1.0 - success
		}

		byID[id] = MCPServerSLORow{
			ServerID:        id,
			CallTotal:       total,
			SuccessTotal:    c.OK,
			TransportErrors: c.TransportErr,
			MCPErrors:       c.MCPError,
			SuccessRatio:    success,
			ErrorRatio:      errRatio,
			TargetRatio:     target,
			TargetMet:       success >= target,
			UpdatedAt:       now,
		}
	}

	for _, s := range catalog {
		row, ok := byID[s.ID]
		if !ok {
			row = MCPServerSLORow{
				ServerID:     s.ID,
				TargetRatio:  target,
				SuccessRatio: 1.0,
				TargetMet:    true,
				UpdatedAt:    now,
			}
		}
		row.ServerName = s.Name
		byID[s.ID] = row
	}

	out := make([]MCPServerSLORow, 0, len(byID))
	for _, row := range byID {
		out = append(out, row)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ServerID < out[j].ServerID
	})

	return out
}

func MCPServerSLODashboardJSON() string {
	rows := MCPServerSLODashboard()
	b, err := json.Marshal(rows)
	if err != nil {
		return "[]"
	}

	return string(b)
}
