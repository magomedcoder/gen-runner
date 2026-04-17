package usecase

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
)

type sendPromptAssemblyInput struct {
	sessionID                int64
	userID                   int
	resolvedModel            string
	settings                 *domain.ChatSessionSettings
	history                  []*domain.Message
	userMessage              string
	userMsg                  *domain.Message
	attachmentFileIDs        []int64
	attachmentNames          []string
	attachmentContents       [][]byte
	fileRAG                  *SendMessageFileRAGOptions
	preferFullDocumentIfFits bool
}

func (c *ChatUseCase) buildSendPromptAssembly(ctx context.Context, in sendPromptAssemblyInput) ([]*domain.Message, *ragStreamMeta, error) {
	systemPolicy := c.llmChatSystemMessage(ctx, in.sessionID, in.settings, in.userID)
	systemAndHistory := make([]*domain.Message, 0, len(in.history)+1)
	systemAndHistory = append(systemAndHistory, systemPolicy)
	systemAndHistory = append(systemAndHistory, in.history...)

	userMsgForLLM := *in.userMsg
	userMsgForLLM.Content = in.userMessage
	userInstruction := &userMsgForLLM
	if len(in.attachmentContents) == 1 && document.IsImageAttachment(in.attachmentNames[0]) {
		userInstruction.AttachmentName = in.attachmentNames[0]
		userInstruction.AttachmentContent = in.attachmentContents[0]
	}

	documentContextBlocks, ragStream, err := c.buildDocumentContextPipeline(ctx, in, systemAndHistory)
	if err != nil {
		return nil, nil, err
	}

	documentContextBlocks, budgetMetrics := c.applyInstructionSafeBudgetManager(systemPolicy, in.history, userInstruction, documentContextBlocks)
	if budgetMetrics.DroppedRunesTotal > 0 {
		logger.I(
			"ChatUseCase: instruction-safe budget manager отбросил %d символов контекста (by_source=%v by_file=%v)",
			budgetMetrics.DroppedRunesTotal,
			budgetMetrics.DroppedRunesBySource,
			budgetMetrics.DroppedRunesByFile,
		)
	}

	if len(documentContextBlocks) > 0 {
		userInstruction.AttachmentFileID = nil
		userInstruction.AttachmentName = ""
		userInstruction.AttachmentContent = nil
	}

	messagesForLLM := assemblePromptMessages(in.sessionID, systemPolicy, in.history, userInstruction, documentContextBlocks)
	return messagesForLLM, ragStream, nil
}

func (c *ChatUseCase) buildDocumentContextPipeline(
	ctx context.Context,
	in sendPromptAssemblyInput,
	systemAndHistory []*domain.Message,
) ([]documentContextBlock, *ragStreamMeta, error) {
	documentContextBlocks := make([]documentContextBlock, 0, len(in.attachmentContents))
	var ragStream *ragStreamMeta
	if len(in.attachmentContents) == 0 {
		return documentContextBlocks, nil, nil
	}

	if len(in.attachmentContents) > 1 {
		for i := range in.attachmentContents {
			if document.IsImageAttachment(in.attachmentNames[i]) {
				return nil, nil, fmt.Errorf("мультивложение не поддерживает изображения: отправьте изображение отдельным сообщением")
			}
		}
	}

	if len(in.attachmentContents) == 1 && document.IsImageAttachment(in.attachmentNames[0]) {
		if in.fileRAG != nil && in.fileRAG.UseFileRAG {
			return nil, nil, fmt.Errorf("режим use_file_rag не поддерживает изображения")
		}
		return documentContextBlocks, nil, nil
	}

	if len(in.attachmentContents) == 1 && in.fileRAG != nil && in.fileRAG.UseFileRAG {
		fid := in.attachmentFileIDs[0]
		query := strings.TrimSpace(in.userMessage)
		if query == "" {
			query = "Ответь по содержимому прикреплённого документа."
		}

		ragBudget := c.effectiveMaxRAGContextRunes(systemAndHistory, in.userMessage)
		useFullText := in.preferFullDocumentIfFits && !in.fileRAG.ForceVectorSearch
		if useFullText {
			extracted, err := document.ExtractText(in.attachmentNames[0], in.attachmentContents[0])
			if err != nil {
				return nil, nil, err
			}

			documentContextBlocks = append(documentContextBlocks, buildAttachmentContextBlock(in.attachmentNames[0], extracted, ragBudget))
			if rm, err := buildRAGStreamMetaFullDocument(fid, extracted); err != nil {
				logger.W("ChatUseCase: метаданные стрима RAG: %v", err)
			} else {
				ragStream = rm
			}

			return documentContextBlocks, ragStream, nil
		}

		if c.documentIngest == nil {
			return nil, nil, domain.ErrRAGNotConfigured
		}

		idx, err := c.documentIngest.GetIngestionStatus(ctx, in.userID, in.sessionID, fid)
		if err != nil {
			return nil, nil, err
		}

		if idx != nil && idx.Status == domain.FileRAGIndexStatusFailed {
			msg := strings.TrimSpace(idx.LastError)
			if msg == "" {
				msg = "см. журнал сервера"
			}
			return nil, nil, fmt.Errorf("%w: %s", domain.ErrRAGIndexFailed, msg)
		}

		if idx == nil || idx.Status != domain.FileRAGIndexStatusReady {
			st := "нет записи"
			if idx != nil {
				st = idx.Status
			}
			return nil, nil, fmt.Errorf("%w: статус=%s", domain.ErrRAGIndexNotReady, st)
		}

		topK := in.fileRAG.TopK
		if topK <= 0 {
			topK = defaultFileRAGTopK
		}

		if topK > maxFileRAGTopK {
			topK = maxFileRAGTopK
		}

		scored, err := c.documentIngest.SearchSessionKnowledge(ctx, in.userID, in.sessionID, in.fileRAG.EmbedModel, query, topK, &fid, c.ragNeighborChunkWindow)
		if err != nil {
			return nil, nil, err
		}

		if len(scored) == 0 {
			return nil, nil, domain.ErrRAGNoHits
		}

		var deepSummary string
		var deepMapCalls int
		if c.deepRAGEnabled {
			ds, nMap, dms, derr := c.deepRAGMapSummaries(ctx, in.sessionID, query, scored, in.resolvedModel)
			deepMapCalls = nMap
			if derr != nil {
				logger.W("ChatUseCase: deep_rag: %v", derr)
			} else if nMap > 0 {
				deepSummary = ds
				logger.I("ChatUseCase: deep_rag вызовов_map=%d длительность_мс=%d символов_вывода=%d", nMap, dms, utf8.RuneCountInString(ds))
			}
		}

		block, droppedByBudget := buildRAGContextBlock(in.attachmentNames[0], scored, ragBudget, deepSummary)
		documentContextBlocks = append(documentContextBlocks, block)
		if rm, err := buildRAGStreamMetaVector(fid, topK, c.ragNeighborChunkWindow, scored, deepMapCalls, strings.TrimSpace(deepSummary) != "", droppedByBudget); err != nil {
			logger.W("ChatUseCase: метаданные стрима RAG: %v", err)
		} else {
			ragStream = rm
		}

		return documentContextBlocks, ragStream, nil
	}

	query := strings.TrimSpace(in.userMessage)
	if query == "" {
		query = "Ответь по содержимому прикреплённых документов."
	}

	totalBudget := c.effectiveMaxRAGContextRunes(systemAndHistory, in.userMessage)
	perFileBudget := max(totalBudget/max(len(in.attachmentContents), 1), 240)
	blocks, err := c.buildMultiFileRetrievalContext(ctx, in, query, totalBudget, perFileBudget)
	if err != nil {
		return nil, nil, err
	}
	documentContextBlocks = append(documentContextBlocks, blocks...)

	return documentContextBlocks, ragStream, nil
}
