package usecase

import (
	"context"
	"sort"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
)

const (
	multiFileRAGSearchTopK   = 6
	multiFileRAGPerFileLimit = 3
	multiFileRAGTotalLimit   = 10
)

type multiFileRAGCandidate struct {
	fileIndex int
	score     float64
	chunk     domain.ScoredDocumentRAGChunk
}

func (c *ChatUseCase) buildMultiFileRetrievalContext(
	ctx context.Context,
	in sendPromptAssemblyInput,
	query string,
	totalBudget int,
	perFileBudget int,
) ([]documentContextBlock, error) {
	if c.documentIngest == nil {
		return nil, domain.ErrRAGNotConfigured
	}

	candidates := make([]multiFileRAGCandidate, 0, len(in.attachmentFileIDs)*multiFileRAGSearchTopK)
	for i, fid := range in.attachmentFileIDs {
		idx, err := c.documentIngest.GetIngestionStatus(ctx, in.userID, in.sessionID, fid)
		if err != nil || idx == nil || idx.Status != domain.FileRAGIndexStatusReady {
			continue
		}

		scored, err := c.documentIngest.SearchSessionKnowledge(ctx, in.userID, in.sessionID, "", query, multiFileRAGSearchTopK, &fid, c.ragNeighborChunkWindow)
		if err != nil {
			continue
		}

		for _, sc := range scored {
			candidates = append(candidates, multiFileRAGCandidate{
				fileIndex: i,
				score:     sc.Score,
				chunk:     sc,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, domain.ErrRAGNoHits
	}

	selected := selectMultiFileRAGCandidates(candidates, totalBudget, perFileBudget)
	if len(selected) == 0 {
		return nil, domain.ErrRAGNoHits
	}

	blocks := make([]documentContextBlock, 0, len(selected))
	for i, name := range in.attachmentNames {
		scored := selected[i]
		if len(scored) == 0 {
			continue
		}

		block, _ := buildRAGContextBlock(name, scored, perFileBudget, "")
		blocks = append(blocks, block)
	}

	return blocks, nil
}

func selectMultiFileRAGCandidates(
	candidates []multiFileRAGCandidate,
	totalBudget int,
	perFileBudget int,
) map[int][]domain.ScoredDocumentRAGChunk {
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	perFileCounts := make(map[int]int, len(candidates))
	perFileRunes := make(map[int]int, len(candidates))
	selected := make(map[int][]domain.ScoredDocumentRAGChunk, len(candidates))
	selectedTotal := 0
	usedTotalRunes := 0
	for _, cand := range candidates {
		if selectedTotal >= multiFileRAGTotalLimit {
			break
		}

		if perFileCounts[cand.fileIndex] >= multiFileRAGPerFileLimit {
			continue
		}

		chunkRunes := utf8.RuneCountInString(cand.chunk.DocumentRAGChunk.Text)
		if perFileRunes[cand.fileIndex]+chunkRunes > perFileBudget {
			continue
		}

		if usedTotalRunes+chunkRunes > totalBudget {
			continue
		}

		perFileCounts[cand.fileIndex]++
		perFileRunes[cand.fileIndex] += chunkRunes
		usedTotalRunes += chunkRunes
		selectedTotal++
		selected[cand.fileIndex] = append(selected[cand.fileIndex], cand.chunk)
	}

	return selected
}
