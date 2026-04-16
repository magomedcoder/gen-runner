package mcpclient

import (
	"strings"
	"testing"
)

func TestMCPServerSLODashboardIncludesActiveServersWithoutTraffic(t *testing.T) {
	SetActiveServerCatalogProvider(func() []ActiveServerDescriptor {
		return []ActiveServerDescriptor{
			{
				ID:   777,
				Name: "idle",
			},
		}
	})
	t.Cleanup(func() { SetActiveServerCatalogProvider(nil) })

	rows := MCPServerSLODashboard()
	if len(rows) == 0 {
		t.Fatal("expected non-empty dashboard")
	}

	var found bool
	for _, r := range rows {
		if r.ServerID != 777 {
			continue
		}

		found = true
		if r.CallTotal != 0 || !r.TargetMet {
			t.Fatalf("unexpected idle row: %+v", r)
		}
	}

	if !found {
		t.Fatal("active server from catalog is missing in dashboard")
	}
}

func TestWritePrometheusMetricsIncludesSLOMetrics(t *testing.T) {
	SetActiveServerCatalogProvider(func() []ActiveServerDescriptor {
		return []ActiveServerDescriptor{
			{ID: 42, Name: "prod-42"},
		}
	})
	t.Cleanup(func() { SetActiveServerCatalogProvider(nil) })

	recordCallToolServer(42, "ok")
	recordCallToolServer(42, "transport_err")

	var b strings.Builder
	if err := WritePrometheusMetrics(&b); err != nil {
		t.Fatal(err)
	}

	out := b.String()
	if !strings.Contains(out, "gen_mcp_server_slo_met") {
		t.Fatalf("missing SLO metric in output: %s", out)
	}

	if !strings.Contains(out, `server_id="42"`) {
		t.Fatalf("missing server_id label: %s", out)
	}
}
