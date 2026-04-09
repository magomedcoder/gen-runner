package usecase

import (
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mcpclient"
)

func TestMCPToolAliasDoesNotNormalizeIntoWebSearch(t *testing.T) {
	t.Parallel()
	builtin := []domain.Tool{
		{
			Name: "web_search",
		},
	}
	allowed := allowedToolNameSet(builtin)
	alias := mcpclient.ToolAlias(7, "search")
	n := normalizeToolName(alias)
	if _, dup := allowed[n]; dup {
		t.Fatalf("алиас %q нормализуется в %q и не должен пересекаться с web_search", alias, n)
	}
}

func TestDistinctMCPServersSameDeclaredToolNameYieldDistinctAliases(t *testing.T) {
	t.Parallel()
	a := normalizeToolName(mcpclient.ToolAlias(1, "ping"))
	b := normalizeToolName(mcpclient.ToolAlias(2, "ping"))
	if a == b {
		t.Fatal("разные server_id должны давать разные алиасы")
	}

	merged := allowedToolNameSet([]domain.Tool{
		{Name: mcpclient.ToolAlias(1, "ping")},
	})
	if _, dup := merged[b]; dup {
		t.Fatal("второй алиас не должен совпадать с первым после нормализации")
	}
}
