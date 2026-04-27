package chatui

import (
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
)

func msg(role domain.MessageRole, id int64, content, toolCallsJSON string) *domain.Message {
	return &domain.Message{
		Id:            id,
		Role:          role,
		Content:       content,
		ToolCallsJSON: toolCallsJSON,
	}
}

func TestPartitionMessagesForToolChainUI_sequentialTwoRoundsAndFinal(t *testing.T) {
	tc := `[{"id":"call_1","type":"function","function":{"name":"x","arguments":"{}"}}]`
	msgs := []*domain.Message{
		msg(domain.MessageRoleAssistant, 1, "r1", tc),
		msg(domain.MessageRoleTool, 2, "t1", ""),
		msg(domain.MessageRoleAssistant, 3, "r2", tc),
		msg(domain.MessageRoleTool, 4, "t2", ""),
		msg(domain.MessageRoleAssistant, 5, "final", ""),
	}

	part := PartitionMessagesForToolChainUI(msgs)
	if len(part) != 1 {
		t.Fatalf("expected 1 partition element, got %d", len(part))
	}

	if part[0].SingleIndex != nil || part[0].Chain == nil {
		t.Fatalf("expected chain group")
	}

	ch := part[0].Chain
	if len(ch.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(ch.Segments))
	}

	if ch.Segments[0].LeadIndex != 0 || ch.Segments[0].ToolStart != 1 || ch.Segments[0].ToolEnd != 1 {
		t.Fatalf("segment0: %+v", ch.Segments[0])
	}

	if ch.Segments[1].LeadIndex != 2 || ch.Segments[1].ToolStart != 3 || ch.Segments[1].ToolEnd != 3 {
		t.Fatalf("segment1: %+v", ch.Segments[1])
	}

	if ch.FinalAssistantIdx == nil || *ch.FinalAssistantIdx != 4 {
		t.Fatalf("final idx: %v", ch.FinalAssistantIdx)
	}
}

func TestPartitionMessagesForToolChainUI_userBetweenBreaks(t *testing.T) {
	tc := `[{"id":"call_1","type":"function","function":{"name":"x","arguments":"{}"}}]`
	msgs := []*domain.Message{
		msg(domain.MessageRoleAssistant, 1, "r1", tc),
		msg(domain.MessageRoleTool, 2, "t1", ""),
		msg(domain.MessageRoleUser, 3, "hi", ""),
		msg(domain.MessageRoleAssistant, 4, "final", ""),
	}

	part := PartitionMessagesForToolChainUI(msgs)
	if len(part) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(part))
	}

	if part[0].Chain == nil || len(part[0].Chain.Segments) != 1 {
		t.Fatal("expected first chain 1 segment")
	}

	if part[0].Chain.FinalAssistantIdx != nil {
		t.Fatal("expected no final assistant in first chain")
	}

	if *part[1].SingleIndex != 2 {
		t.Fatalf("expected user at 2, got %v", part[1].SingleIndex)
	}

	if *part[2].SingleIndex != 3 {
		t.Fatalf("expected assistant at 3, got %v", part[2].SingleIndex)
	}
}

func TestPartitionMessagesForToolChainUI_singleAssistantNoTools(t *testing.T) {
	tc := `[{"id":"call_1","type":"function","function":{"name":"x","arguments":"{}"}}]`
	msgs := []*domain.Message{
		msg(domain.MessageRoleAssistant, 1, "only", tc),
	}

	part := PartitionMessagesForToolChainUI(msgs)
	if len(part) != 1 || part[0].SingleIndex == nil || *part[0].SingleIndex != 0 {
		t.Fatalf("expected single message, got %+v", part)
	}
}
