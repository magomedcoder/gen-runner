package handler

import (
	"testing"

	"github.com/magomedcoder/gen/api/pb/chatpb"
	"github.com/magomedcoder/gen/internal/usecase"
)

func TestStreamSendLoop_AssistantFinalAccumulatesTextAndReasoning(t *testing.T) {
	ch := make(chan usecase.ChatStreamChunk, 4)
	go func() {
		ch <- usecase.ChatStreamChunk{
			Kind:      usecase.StreamChunkKindText,
			Text:      "a",
			MessageID: 42,
		}
		ch <- usecase.ChatStreamChunk{
			Kind:      usecase.StreamChunkKindReasoning,
			Text:      "r1",
			MessageID: 42,
		}
		ch <- usecase.ChatStreamChunk{
			Kind: usecase.StreamChunkKindText,
			Text: "b", MessageID: 42,
		}
		close(ch)
	}()

	var got []*chatpb.ChatResponse
	err := streamSendLoop(ch, func(r *chatpb.ChatResponse) error {
		got = append(got, r)
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if len(got) < 2 {
		t.Fatalf("want at least 2 responses (chunks + final), got %d", len(got))
	}

	last := got[len(got)-1]
	if !last.GetDone() {
		t.Fatal("last response must be done")
	}

	af := last.GetAssistantFinal()
	if af == nil {
		t.Fatal("expected assistant_final on final response")
	}

	if af.GetAssistantMessageId() != 42 {
		t.Fatalf("assistant_message_id: want 42, got %d", af.GetAssistantMessageId())
	}

	if af.GetText() != "ab" {
		t.Fatalf("text: want ab, got %q", af.GetText())
	}

	if af.GetReasoning() != "r1" {
		t.Fatalf("reasoning: want r1, got %q", af.GetReasoning())
	}
}

func TestStreamSendLoop_EmitsToolStatusChunks(t *testing.T) {
	ch := make(chan usecase.ChatStreamChunk, 3)
	go func() {
		ch <- usecase.ChatStreamChunk{
			Kind:     usecase.StreamChunkKindToolStatus,
			Text:     "Выполняется: MCP Demo b24_list_tasks",
			ToolName: "MCP Demo b24_list_tasks",
		}
		ch <- usecase.ChatStreamChunk{
			Kind:      usecase.StreamChunkKindText,
			Text:      "готово",
			MessageID: 7,
		}
		close(ch)
	}()

	var got []*chatpb.ChatResponse
	err := streamSendLoop(ch, func(r *chatpb.ChatResponse) error {
		got = append(got, r)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 3 {
		t.Fatalf("expected tool status + text + final, got=%d", len(got))
	}
	if got[0].GetChunkKind() != chatpb.StreamChunkKind_STREAM_CHUNK_KIND_TOOL_STATUS {
		t.Fatalf("first chunk kind mismatch: %v", got[0].GetChunkKind())
	}
	if got[0].GetContent() == "" {
		t.Fatal("tool status content should not be empty")
	}
	if !got[len(got)-1].GetDone() || got[len(got)-1].GetAssistantFinal() == nil {
		t.Fatal("final response must contain assistant_final")
	}
}
