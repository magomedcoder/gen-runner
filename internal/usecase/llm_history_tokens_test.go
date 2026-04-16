package usecase

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
)

func TestTrimLLMMessagesByApproxTokens_keepsSystemAndTail(t *testing.T) {
	sys := domain.NewMessage(1, "system prompt", domain.MessageRoleSystem)
	u1 := domain.NewMessage(1, strings.Repeat("a", 400), domain.MessageRoleUser)
	a1 := domain.NewMessage(1, strings.Repeat("b", 400), domain.MessageRoleAssistant)
	u2 := domain.NewMessage(1, "last user", domain.MessageRoleUser)

	msgs := []*domain.Message{sys, u1, a1, u2}
	out, trimmed := trimLLMMessagesByApproxTokens(msgs, 80, 1)
	if !trimmed {
		t.Fatal("ожидалась обрезка")
	}

	if len(out) < 2 {
		t.Fatalf("слишком коротко: %d", len(out))
	}

	if out[0] != sys {
		t.Fatal("system должен остаться первым")
	}

	if out[len(out)-1] != u2 {
		t.Fatal("последнее user должно сохраниться")
	}
}

func TestTrimLLMMessagesByApproxTokens_disabled(t *testing.T) {
	m := domain.NewMessage(1, "x", domain.MessageRoleUser)
	msgs := []*domain.Message{domain.NewMessage(1, "s", domain.MessageRoleSystem), m}
	out, trimmed := trimLLMMessagesByApproxTokens(msgs, 0, 1)
	if trimmed || len(out) != 2 {
		t.Fatalf("maxTokens=0: trimmed=%v len=%d", trimmed, len(out))
	}
}

func TestTrimLLMMessagesByApproxTokens_systemAndOneUserKeepsInstruction(t *testing.T) {
	sys := domain.NewMessage(1, "system prompt text", domain.MessageRoleSystem)
	u := domain.NewMessage(1, strings.Repeat("ж", 12000), domain.MessageRoleUser)
	msgs := []*domain.Message{sys, u}
	out, trimmed := trimLLMMessagesByApproxTokens(msgs, 200, 1)
	if !trimmed {
		t.Fatal("ожидалась обрезка при system + одно user (раньше баг: лимит игнорировался)")
	}

	if got := utf8.RuneCountInString(out[1].Content); got != len([]rune(u.Content)) {
		t.Fatalf("последняя инструкция пользователя не должна укорачиваться: runes=%d", got)
	}
}

func TestTrimLLMMessagesByApproxTokensWithDropped_collectsMiddle(t *testing.T) {
	sys := domain.NewMessage(1, "s", domain.MessageRoleSystem)
	u1 := domain.NewMessage(1, strings.Repeat("a", 400), domain.MessageRoleUser)
	a1 := domain.NewMessage(1, strings.Repeat("b", 400), domain.MessageRoleAssistant)
	u2 := domain.NewMessage(1, "tail", domain.MessageRoleUser)
	msgs := []*domain.Message{sys, u1, a1, u2}
	_, trimmed, dropped := trimLLMMessagesByApproxTokensWithDropped(msgs, 80, 1)
	if !trimmed || len(dropped) < 1 {
		t.Fatalf("trimmed=%v dropped=%d", trimmed, len(dropped))
	}
}

func TestInjectSummaryAfterSystem(t *testing.T) {
	sys := domain.NewMessage(1, "base", domain.MessageRoleSystem)
	u := domain.NewMessage(1, "u", domain.MessageRoleUser)
	out := injectSummaryAfterSystem([]*domain.Message{sys, u}, "summary line")
	if len(out) != 2 || out[0] == sys {
		t.Fatal("ожидалась копия system")
	}

	if !strings.Contains(out[0].Content, "summary line") || !strings.Contains(out[0].Content, "base") {
		t.Fatalf("content=%q", out[0].Content)
	}
}
