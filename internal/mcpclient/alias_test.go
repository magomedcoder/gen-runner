package mcpclient

import (
	"strings"
	"testing"
)

func TestToolAliasParseRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []string{
		"read_file",
		"tool/with.chars",
		"привет",
		"a",
		"  trim_me  ",
	}
	for _, name := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			const sid int64 = 42
			alias := ToolAlias(sid, name)
			gotSid, gotName, ok := ParseToolAlias(alias)
			if !ok || gotSid != sid || gotName != name {
				t.Fatalf("alias=%q: ok=%v sid=%d name=%q want sid=%d name=%q", alias, ok, gotSid, gotName, sid, name)
			}
		})
	}
}

func TestParseToolAliasRejectNonMCPNames(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"",
		"web_search",
		"functions.read_file",
		"mcp_1_",
		"mcp_0_h61",
		"mcp_1_h",
		"mcp_x_h61",
	} {
		if _, _, ok := ParseToolAlias(s); ok {
			t.Fatalf("ожидался отказ для %q", s)
		}
	}
}

func TestParseToolAliasInvalidHex(t *testing.T) {
	t.Parallel()
	if _, _, ok := ParseToolAlias("mcp_1_hzz"); ok {
		t.Fatal("ожидался отказ для невалидного hex")
	}
}

func TestToolAliasDistinctServersSameToolName(t *testing.T) {
	t.Parallel()
	a := ToolAlias(1, "echo")
	b := ToolAlias(2, "echo")
	if a == b {
		t.Fatal("алиасы разных серверов не должны совпадать")
	}

	s1, n1, ok1 := ParseToolAlias(a)
	s2, n2, ok2 := ParseToolAlias(b)
	if !ok1 || !ok2 || n1 != "echo" || n2 != "echo" || s1 != 1 || s2 != 2 {
		t.Fatalf("roundtrip: (%d,%q) (%d,%q)", s1, n1, s2, n2)
	}
}

func TestFormatToolAliasIsLowercaseHex(t *testing.T) {
	t.Parallel()
	a := ToolAlias(10, "X")
	if strings.ToLower(a) != a {
		t.Fatalf("ожидался нижний регистр: %q", a)
	}
}
