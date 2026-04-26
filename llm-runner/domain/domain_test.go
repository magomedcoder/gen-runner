package domain

import (
	"testing"

	"github.com/magomedcoder/gen/api/pb/llm-runner/llmrunnerpb"
)

func TestFromProtoRole_ToProtoRole(t *testing.T) {
	tests := []struct {
		proto string
		want  AIChatMessageRole
	}{
		{"system", AIChatMessageRoleSystem},
		{"user", AIChatMessageRoleUser},
		{"assistant", AIChatMessageRoleAssistant},
		{"tool", AIChatMessageRoleTool},
		{"  User  ", AIChatMessageRoleUser},
		{"unknown", AIChatMessageRoleUser},
		{"", AIChatMessageRoleUser},
	}
	for _, tt := range tests {
		got := AIFromProtoRole(tt.proto)
		if got != tt.want {
			t.Errorf("FromProtoRole(%q) = %v, ожидалось %v", tt.proto, got, tt.want)
		}
		if back := AIToProtoRole(got); back != string(tt.want) && tt.proto != "unknown" && tt.proto != "" {
			if (tt.proto == "unknown" || tt.proto == "") && back == "user" {
				continue
			}
			t.Errorf("ToProtoRole(FromProtoRole(%q)) = %q, неожиданное значение", tt.proto, back)
		}
	}
}

func TestMessage_roleAndContent(t *testing.T) {
	m := &AIChatMessage{
		Content: "hi",
		Role:    AIChatMessageRoleUser,
	}
	if m.Role != AIChatMessageRoleUser || m.Content != "hi" {
		t.Errorf("поля сообщения: %+v", m)
	}
}

func TestNewMessage(t *testing.T) {
	m := NewAIChatMessage(1, "text", AIChatMessageRoleUser)
	if m.SessionId != 1 || m.Content != "text" || m.Role != AIChatMessageRoleUser {
		t.Errorf("NewMessage: неверные поля %+v", m)
	}
}

func TestNewMessageWithAttachment(t *testing.T) {
	m := NewAIChatMessageWithAttachment(1, "text", AIChatMessageRoleAssistant, "file.txt", 0)
	if m.AttachmentName != "file.txt" || m.Role != AIChatMessageRoleAssistant {
		t.Errorf("NewMessageWithAttachment: неверные поля %+v", m)
	}
}

func TestNewChatSession(t *testing.T) {
	s := NewAIChatSession(1, "title", "model1")
	if s.UserId != 1 || s.Title != "title" || s.Model != "model1" {
		t.Errorf("NewChatSession: неверные поля %+v", s)
	}
}

func TestAIMessageFromProto_attachmentContent(t *testing.T) {
	n := "pic.png"
	mime := "image/png"
	p := &llmrunnerpb.ChatMessage{
		Role:              "user",
		Content:           "что на картинке",
		CreatedAt:         1,
		AttachmentName:    &n,
		AttachmentMime:    &mime,
		AttachmentContent: []byte{0x89, 0x50},
	}

	m := AIMessageFromProto(p, 7)
	if m.AttachmentName != n || m.AttachmentMime != mime || string(m.AttachmentContent) != string(p.AttachmentContent) {
		t.Fatalf("из proto: %+v", m)
	}

	p2 := AIMessageToProto(m)
	if p2.GetAttachmentName() != n || p2.GetAttachmentMime() != mime || string(p2.GetAttachmentContent()) != string(p.AttachmentContent) {
		t.Fatalf("в proto: %+v", p2)
	}
}
