package mcpclient

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCallToolResultString_TextAndImage(t *testing.T) {
	raw := []byte{0x89, 0x50}
	res := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: "hello",
			},
			&mcp.ImageContent{
				MIMEType: "image/png",
				Data:     raw,
			},
		},
	}

	s := CallToolResultString(res)
	if !strings.Contains(s, "hello") {
		t.Fatalf("expected text, got %q", s)
	}

	if !strings.Contains(s, "image/png") || !strings.Contains(s, "base64") {
		t.Fatalf("expected image marker, got %q", s)
	}
}

func TestResourceLinkToString(t *testing.T) {
	s := resourceLinkToString(&mcp.ResourceLink{
		URI:         "file:///x",
		Name:        "n",
		Description: "d",
		MIMEType:    "text/plain",
	})
	if !strings.Contains(s, "file:///x") {
		t.Fatal(s)
	}
}
