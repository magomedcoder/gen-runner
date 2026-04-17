package domain

import "testing"

func TestValidateMCPServerStructure_transport(t *testing.T) {
	if err := ValidateMCPServerStructure(&MCPServer{
		Transport:      "stdio",
		TimeoutSeconds: 120,
	}); err != nil {
		t.Fatal(err)
	}

	if err := ValidateMCPServerStructure(&MCPServer{
		Transport:      "SSE",
		TimeoutSeconds: 0,
	}); err != nil {
		t.Fatal(err)
	}

	if err := ValidateMCPServerStructure(&MCPServer{
		Transport:      "bad",
		TimeoutSeconds: 1,
	}); err == nil {
		t.Fatal("expected error for bad transport")
	}
}

func TestValidateMCPServerStructure_timeout(t *testing.T) {
	if err := ValidateMCPServerStructure(&MCPServer{
		Transport:      "stdio",
		TimeoutSeconds: 0,
	}); err != nil {
		t.Fatal(err)
	}

	if err := ValidateMCPServerStructure(&MCPServer{
		Transport:      "stdio",
		TimeoutSeconds: 601,
	}); err == nil {
		t.Fatal("expected error for timeout > 600")
	}
}

func TestNormalizeMCPServer(t *testing.T) {
	s := &MCPServer{
		Transport: "  SSE  ",
		Name:      " x ",
	}
	NormalizeMCPServer(s)
	if s.Transport != "sse" || s.Name != "x" {
		t.Fatalf("got transport=%q name=%q", s.Transport, s.Name)
	}
}
