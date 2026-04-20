package usecase

import (
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/mcpclient"
)

func TestResolveDeclaredToolNameBareMCPMatchesHexAlias(t *testing.T) {
	orig := "b24_list_tasks"
	alias := mcpclient.ToolAlias(1, orig)
	gp := &domain.GenerationParams{
		Tools: []domain.Tool{
			{
				Name:           alias,
				Description:    "x",
				ParametersJSON: `{}`,
			},
			{
				Name:           "web_search",
				ParametersJSON: `{}`,
			},
		},
	}

	got, ok := resolveDeclaredToolName(gp, orig)
	if !ok || got != normalizeToolName(alias) {
		t.Fatalf("resolve bare name: ok=%v got=%q want=%q", ok, got, normalizeToolName(alias))
	}

	got2, ok2 := resolveDeclaredToolName(gp, alias)
	if !ok2 || got2 != normalizeToolName(alias) {
		t.Fatalf("resolve full alias: ok=%v got=%q", ok2, got2)
	}
}

func TestResolveDeclaredToolNameAmbiguousBarePicksLowerServerID(t *testing.T) {
	orig := "ping"
	a1 := mcpclient.ToolAlias(1, orig)
	a2 := mcpclient.ToolAlias(2, orig)
	gp := &domain.GenerationParams{
		Tools: []domain.Tool{
			{
				Name:           a2,
				ParametersJSON: `{}`,
			},
			{
				Name:           a1,
				ParametersJSON: `{}`,
			},
		},
	}

	got, ok := resolveDeclaredToolName(gp, orig)
	if !ok || got != normalizeToolName(a1) {
		t.Fatalf("want server 1 first, got ok=%v %q", ok, got)
	}
}

func TestResolveDeclaredToolNameFullAliasKeepsRequestedServer(t *testing.T) {
	orig := "ping"
	a1 := mcpclient.ToolAlias(1, orig)
	a2 := mcpclient.ToolAlias(2, orig)
	gp := &domain.GenerationParams{
		Tools: []domain.Tool{
			{
				Name:           a1,
				ParametersJSON: `{}`,
			},
			{
				Name:           a2,
				ParametersJSON: `{}`,
			},
		},
	}

	got, ok := resolveDeclaredToolName(gp, a2)
	if !ok || got != normalizeToolName(a2) {
		t.Fatalf("full alias must keep exact server alias: ok=%v got=%q want=%q", ok, got, normalizeToolName(a2))
	}
}

func TestResolveDeclaredToolNameUnknown(t *testing.T) {
	gp := &domain.GenerationParams{
		Tools: []domain.Tool{
			{
				Name:           mcpclient.ToolAlias(1, "x"),
				ParametersJSON: `{}`,
			},
		},
	}

	if _, ok := resolveDeclaredToolName(gp, "nonexistent_tool_xyz"); ok {
		t.Fatal("expected no match")
	}
}
