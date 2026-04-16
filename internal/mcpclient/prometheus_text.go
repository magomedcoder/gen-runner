package mcpclient

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

func WritePrometheusMetrics(w io.Writer) error {
	m := MCPCountersMap()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	for _, k := range keys {
		name := prometheusMetricName(k)
		fmt.Fprintf(w, "# TYPE %s counter\n", name)
		fmt.Fprintf(w, "%s %d\n", name, m[k])
	}

	writeCallToolDurationHistogram(w)
	writeServerSLOMetrics(w)

	return nil
}

func promLabelEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func writeServerSLOMetrics(w io.Writer) {
	rows := MCPServerSLODashboard()
	if len(rows) == 0 {
		return
	}

	fmt.Fprintf(w, "# HELP gen_mcp_server_call_tool_total Всего tools/call по MCP server_id\n")
	fmt.Fprintf(w, "# TYPE gen_mcp_server_call_tool_total gauge\n")
	fmt.Fprintf(w, "# HELP gen_mcp_server_transport_error_total Transport/timeout ошибки по MCP server_id\n")
	fmt.Fprintf(w, "# TYPE gen_mcp_server_transport_error_total gauge\n")
	fmt.Fprintf(w, "# HELP gen_mcp_server_success_ratio Доля успешных tools/call (0..1)\n")
	fmt.Fprintf(w, "# TYPE gen_mcp_server_success_ratio gauge\n")
	fmt.Fprintf(w, "# HELP gen_mcp_server_slo_target_ratio Целевой SLO availability (0..1)\n")
	fmt.Fprintf(w, "# TYPE gen_mcp_server_slo_target_ratio gauge\n")
	fmt.Fprintf(w, "# HELP gen_mcp_server_slo_met Выполнение SLO (1=ok,0=breach)\n")
	fmt.Fprintf(w, "# TYPE gen_mcp_server_slo_met gauge\n")

	for _, row := range rows {
		labels := fmt.Sprintf(`server_id="%d",server_name="%s"`, row.ServerID, promLabelEscape(strings.TrimSpace(row.ServerName)))
		fmt.Fprintf(w, "gen_mcp_server_call_tool_total{%s} %d\n", labels, row.CallTotal)
		fmt.Fprintf(w, "gen_mcp_server_transport_error_total{%s} %d\n", labels, row.TransportErrors)
		fmt.Fprintf(w, "gen_mcp_server_success_ratio{%s} %g\n", labels, row.SuccessRatio)
		fmt.Fprintf(w, "gen_mcp_server_slo_target_ratio{%s} %g\n", labels, row.TargetRatio)
		if row.TargetMet {
			fmt.Fprintf(w, "gen_mcp_server_slo_met{%s} 1\n", labels)
		} else {
			fmt.Fprintf(w, "gen_mcp_server_slo_met{%s} 0\n", labels)
		}
	}
}

func prometheusMetricName(key string) string {
	var b strings.Builder
	b.WriteString("gen_mcp_")
	for _, r := range key {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == '_':
			b.WriteRune('_')
		default:
			b.WriteRune('_')
		}
	}

	return b.String()
}
