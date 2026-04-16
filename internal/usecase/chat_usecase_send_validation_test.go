package usecase

import "testing"

func TestNormalizeAttachmentFileIDsForModel(t *testing.T) {
	t.Run("deduplicate and preserve order", func(t *testing.T) {
		got, err := normalizeAttachmentFileIDsForModel([]int64{10, 20, 10, 30})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 || got[0] != 10 || got[1] != 20 || got[2] != 30 {
			t.Fatalf("unexpected normalized ids: %#v", got)
		}
	})

	t.Run("reject non-positive ids", func(t *testing.T) {
		if _, err := normalizeAttachmentFileIDsForModel([]int64{1, 0}); err == nil {
			t.Fatal("expected error for non-positive attachment id")
		}
	})

	t.Run("reject more than max attachments", func(t *testing.T) {
		ids := []int64{1, 2, 3, 4, 5}
		if _, err := normalizeAttachmentFileIDsForModel(ids); err == nil {
			t.Fatal("expected error for too many attachments")
		}
	})
}

func TestValidateFileRAGOptions(t *testing.T) {
	t.Run("allow nil options", func(t *testing.T) {
		if err := validateFileRAGOptions(nil, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject file_rag params when use_file_rag=false", func(t *testing.T) {
		err := validateFileRAGOptions(&SendMessageFileRAGOptions{
			UseFileRAG: false,
			TopK:       5,
		}, []int64{1})
		if err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("require exactly one attachment for use_file_rag", func(t *testing.T) {
		err := validateFileRAGOptions(&SendMessageFileRAGOptions{
			UseFileRAG: true,
		}, []int64{1, 2})
		if err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("reject negative top_k", func(t *testing.T) {
		err := validateFileRAGOptions(&SendMessageFileRAGOptions{
			UseFileRAG: true,
			TopK:       -1,
		}, []int64{1})
		if err == nil {
			t.Fatal("expected validation error")
		}
	})
}
