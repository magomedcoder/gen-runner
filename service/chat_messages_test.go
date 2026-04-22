package service

import (
	"strings"
	"testing"

	"github.com/magomedcoder/gen-runner/domain"
)

func TestFormatContentForBuiltinChatTemplate_toolRoleLongMCPStyleContent(t *testing.T) {
	t.Parallel()
	const n = 200_000
	long := strings.Repeat("Z", n)
	m := &domain.AIChatMessage{
		Role:       domain.AIChatMessageRoleTool,
		Content:    long,
		ToolCallID: "call_abc",
		ToolName:   "mcp_3_h636174",
	}

	got := FormatContentForBuiltinChatTemplate(m)
	if !strings.HasPrefix(got, "[call_id=call_abc] ") {
		prefixLen := 40
		if len(got) < prefixLen {
			prefixLen = len(got)
		}
		t.Fatalf("prefix: %q", got[:prefixLen])
	}

	if !strings.Contains(got, "[mcp_3_h636174] ") {
		t.Fatal("ожидалось имя инструмента в префиксе")
	}

	if len(got) != len("[call_id=call_abc] [mcp_3_h636174] ")+n {
		t.Fatalf("длина: got %d want %d", len(got), len("[call_id=call_abc] [mcp_3_h636174] ")+n)
	}

	if !strings.HasSuffix(got, "ZZZ") {
		t.Fatal("хвост контента обрезан")
	}
}

func TestNormalizeChatMessages_preservesUserInstructionAsLastMessage(t *testing.T) {
	in := []*domain.AIChatMessage{
		domain.NewAIChatMessage(1, "system-a", domain.AIChatMessageRoleSystem),
		domain.NewAIChatMessage(1, "system-b", domain.AIChatMessageRoleSystem),
		domain.NewAIChatMessage(1, "history answer", domain.AIChatMessageRoleAssistant),
		domain.NewAIChatMessage(1, "strict output format", domain.AIChatMessageRoleUser),
	}

	out := NormalizeChatMessages(in)
	if len(out) != 3 {
		t.Fatalf("len=%d", len(out))
	}

	if out[0].Role != domain.AIChatMessageRoleSystem {
		t.Fatalf("first role must be system, got %s", out[0].Role)
	}

	if out[len(out)-1].Role != domain.AIChatMessageRoleUser || out[len(out)-1].Content != "strict output format" {
		t.Fatalf("last message must be latest user instruction, got role=%s content=%q", out[len(out)-1].Role, out[len(out)-1].Content)
	}
}

func TestMessagesHaveVisionAttachments_imageMimeOrFilename(t *testing.T) {
	if !MessagesHaveVisionAttachments([]*domain.AIChatMessage{
		{
			AttachmentMime:    "image/png",
			AttachmentContent: []byte{1},
		},
	}) {
		t.Fatal("image/* mime must count as vision payload")
	}

	if MessagesHaveVisionAttachments([]*domain.AIChatMessage{
		{
			AttachmentMime:    "application/pdf",
			AttachmentName:    "x.pdf",
			AttachmentContent: []byte{1, 2, 3},
		},
	}) {
		t.Fatal("non-image mime must not count as vision")
	}

	if !MessagesHaveVisionAttachments([]*domain.AIChatMessage{
		{
			AttachmentName:    "pic.png",
			AttachmentContent: []byte{1},
		},
	}) {
		t.Fatal("legacy: image extension without mime still treated as vision candidate")
	}
}
