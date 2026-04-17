package usecase

import (
	"fmt"
	"strings"
)

const maxAttachmentsPerMessage = 4

type SendMessageFileRAGOptions struct {
	UseFileRAG        bool
	TopK              int
	EmbedModel        string
	ForceVectorSearch bool
}

func normalizeAttachmentFileIDsForModel(ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	normalized := make([]int64, 0, len(ids))
	for _, fid := range ids {
		if fid <= 0 {
			return nil, fmt.Errorf("некорректный attachment_file_id: %d", fid)
		}

		duplicate := false
		for _, existing := range normalized {
			if existing == fid {
				duplicate = true
				break
			}
		}

		if !duplicate {
			normalized = append(normalized, fid)
		}
	}

	if len(normalized) > maxAttachmentsPerMessage {
		return nil, fmt.Errorf("слишком много вложений: максимум %d на сообщение", maxAttachmentsPerMessage)
	}

	return normalized, nil
}

func validateFileRAGOptions(fileRAG *SendMessageFileRAGOptions, attachmentFileIDs []int64) error {
	if fileRAG == nil {
		return nil
	}

	if !fileRAG.UseFileRAG {
		if fileRAG.TopK != 0 || strings.TrimSpace(fileRAG.EmbedModel) != "" || fileRAG.ForceVectorSearch {
			return fmt.Errorf("параметры file_rag_* допустимы только при use_file_rag=true")
		}

		return nil
	}

	if len(attachmentFileIDs) != 1 {
		return fmt.Errorf("режим use_file_rag требует ровно один attachment_file_id")
	}

	if fileRAG.TopK < 0 {
		return fmt.Errorf("file_rag_top_k не может быть отрицательным")
	}

	return nil
}

func normalizeAttachmentHydrateParallelism(n int) int {
	if n <= 0 {
		return 8
	}

	if n > 64 {
		return 64
	}

	return n
}
