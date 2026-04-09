package mcpclient

import (
	"strings"
	"testing"
)

func TestRedactMCPLogMessagePayloadBearer(t *testing.T) {
	in := `Authorization: Bearer 11`
	got := redactMCPLogMessagePayload(in)
	if strings.Contains(got, "eyJ") {
		t.Fatalf("token leaked: %q", got)
	}

	if !strings.Contains(got, "bearer [REDACTED]") {
		t.Fatalf("expected redaction marker: %q", got)
	}
}

func TestRedactMCPLogMessagePayloadCaseInsensitive(t *testing.T) {
	in := "BEARER AAAAAAAAAAAAAAAA"
	got := redactMCPLogMessagePayload(in)
	if strings.Contains(strings.ToUpper(got), "AAAAAAAA") {
		t.Fatalf("got %q", got)
	}
}

func TestFormatLoggingMessageDataRedactsJSON(t *testing.T) {
	got := formatLoggingMessageData(map[string]any{
		"msg": "token Bearer abcdefghijklmnop",
	})

	if strings.Contains(got, "abcdefghijklmnop") {
		t.Fatalf("got %q", got)
	}

	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("got %q", got)
	}
}
