package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
	"github.com/magomedcoder/gen/pkg/rag"
)

type DocumentIngestUseCase struct {
	sessionRepo        domain.ChatSessionRepository
	fileRepo           domain.FileRepository
	ragRepo            domain.DocumentRAGRepository
	runnerRepo         domain.RunnerRepository
	llmRepo            domain.LLMRepository
	attachmentsSaveDir string
	splitOpts          rag.SplitOptions
	embedBatchSize     int
	maxChunkEmbedRunes int
}

func NewDocumentIngestUseCase(
	sessionRepo domain.ChatSessionRepository,
	fileRepo domain.FileRepository,
	ragRepo domain.DocumentRAGRepository,
	runnerRepo domain.RunnerRepository,
	llmRepo domain.LLMRepository,
	attachmentsSaveDir string,
	splitOpts rag.SplitOptions,
	embedBatchSize int,
	maxChunkEmbedRunes int,
) *DocumentIngestUseCase {
	if splitOpts.ChunkSizeRunes <= 0 {
		splitOpts.ChunkSizeRunes = 1024
	}

	if splitOpts.ChunkOverlapRunes < 0 {
		splitOpts.ChunkOverlapRunes = 0
	}

	if embedBatchSize <= 0 {
		embedBatchSize = 32
	}

	if maxChunkEmbedRunes <= 0 {
		maxChunkEmbedRunes = 8192
	}

	return &DocumentIngestUseCase{
		sessionRepo:        sessionRepo,
		fileRepo:           fileRepo,
		ragRepo:            ragRepo,
		runnerRepo:         runnerRepo,
		llmRepo:            llmRepo,
		attachmentsSaveDir: attachmentsSaveDir,
		splitOpts:          splitOpts,
		embedBatchSize:     embedBatchSize,
		maxChunkEmbedRunes: maxChunkEmbedRunes,
	}
}

func (u *DocumentIngestUseCase) IndexSessionFile(ctx context.Context, userID int, sessionID, fileID int64, requestedEmbeddingModel string) error {
	ingestStart := time.Now()
	if strings.TrimSpace(u.attachmentsSaveDir) == "" {
		return fmt.Errorf("хранилище вложений не настроено")
	}

	session, err := u.sessionRepo.GetById(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.UserId != userID {
		return domain.ErrUnauthorized
	}

	f, err := u.fileRepo.GetByIdWithExtractedCache(ctx, fileID)
	if err != nil {
		return fmt.Errorf("файл id=%d: %w", fileID, err)
	}

	if f == nil {
		return fmt.Errorf("файл id=%d не найден", fileID)
	}

	if f.ChatSessionID == nil || *f.ChatSessionID != sessionID {
		return fmt.Errorf("файл не относится к этой сессии")
	}

	if f.UserID == nil || *f.UserID != userID {
		return fmt.Errorf("файл не принадлежит пользователю")
	}

	if f.ExpiresAt != nil && time.Now().After(*f.ExpiresAt) {
		return fmt.Errorf("срок действия файла истёк")
	}

	baseName := filepath.Base(strings.TrimSpace(f.Filename))
	if baseName == "" || baseName == "." {
		baseName = "file"
	}

	if document.IsImageAttachment(baseName) {
		return fmt.Errorf("индексация RAG для изображений не поддерживается")
	}

	sessionModel := ""
	if session.SelectedRunnerID != nil {
		ru, rerr := u.runnerRepo.GetByID(ctx, *session.SelectedRunnerID)
		if rerr == nil && ru != nil {
			sessionModel = strings.TrimSpace(ru.SelectedModel)
		}
	}

	embedModel, err := resolveModelForUser(ctx, u.llmRepo, strings.TrimSpace(requestedEmbeddingModel), sessionModel)
	if err != nil {
		return err
	}

	path := strings.TrimSpace(f.StoragePath)
	if path == "" {
		return fmt.Errorf("файл id=%d: пустой storage_path", fileID)
	}

	expectedDir := filepath.Clean(filepath.Join(u.attachmentsSaveDir, strconv.FormatInt(sessionID, 10)))
	cleanPath := filepath.Clean(path)
	if !strings.HasPrefix(cleanPath, expectedDir+string(filepath.Separator)) && cleanPath != expectedDir {
		return fmt.Errorf("файл id=%d: неверный путь хранения", fileID)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("чтение файла: %w", err)
	}

	if len(data) > document.MaxRecommendedAttachmentSizeBytes {
		return fmt.Errorf("размер вложения превышает лимит %d байт", document.MaxRecommendedAttachmentSizeBytes)
	}

	if err := document.ValidateAttachment(baseName, data); err != nil {
		return err
	}

	sum := sha256.Sum256(data)
	shaHex := hex.EncodeToString(sum[:])

	extractPhase := time.Now()
	var extracted string
	if strings.EqualFold(f.ExtractedTextContentSha256, shaHex) && strings.TrimSpace(f.ExtractedText) != "" {
		extracted = f.ExtractedText
	} else {
		extracted, err = document.ExtractText(baseName, data)
		if err != nil {
			logger.W("DocumentIngest: извлечение текста %q: %v", baseName, err)
			return fmt.Errorf("%w: %v", document.ErrTextExtractionFailed, err)
		}

		if len(extracted) <= maxFileExtractedTextCacheBytes {
			if serr := u.fileRepo.SaveExtractedTextCache(ctx, f.Id, shaHex, extracted); serr != nil {
				logger.W("DocumentIngest: кэш текста file_id=%d: %v", f.Id, serr)
			}
		}
	}

	norm := document.NormalizeExtractedText(extracted)
	extractDur := time.Since(extractPhase)
	if strings.TrimSpace(norm) == "" {
		return fmt.Errorf("пустой текст после извлечения и нормализации")
	}

	normSum := sha256.Sum256([]byte(norm))
	sourceHash := hex.EncodeToString(normSum[:])

	if prev, _ := u.ragRepo.GetFileIndex(ctx, fileID); prev != nil &&
		prev.Status == domain.FileRAGIndexStatusReady &&
		prev.SourceContentSHA256 == sourceHash &&
		prev.PipelineVersion == domain.RAGPipelineVersion &&
		prev.EmbeddingModel == embedModel {
		logger.I("DocumentIngest: пропуск (уже готов) file_id=%d session_id=%d model=%q hash=%s за %s",
			fileID, sessionID, embedModel, sourceHash[:8], time.Since(ingestStart).Truncate(time.Millisecond))
		return nil
	}

	now := time.Now()
	if err := u.ragRepo.SaveFileRAGIndex(ctx, &domain.FileRAGIndex{
		FileID:              fileID,
		Status:              domain.FileRAGIndexStatusIndexing,
		LastError:           "",
		SourceContentSHA256: "",
		PipelineVersion:     domain.RAGPipelineVersion,
		EmbeddingModel:      embedModel,
		ChunkCount:          0,
		UpdatedAt:           now,
	}); err != nil {
		return err
	}

	chunkPhase := time.Now()
	rawChunks := rag.SplitText(baseName, norm, u.splitOpts)
	chunkDur := time.Since(chunkPhase)
	if len(rawChunks) == 0 {
		_ = u.ragRepo.MarkFileRAGIndexFailed(ctx, fileID, "не удалось построить чанки")
		return fmt.Errorf("чанки: пустой результат")
	}

	textsForEmbed := make([]string, len(rawChunks))
	for i := range rawChunks {
		t, _ := document.TruncateExtractedText(rawChunks[i].Text, u.maxChunkEmbedRunes)
		textsForEmbed[i] = t
	}

	embedPhase := time.Now()
	allVec, berr := embedTextsBatches(ctx, u.llmRepo, embedModel, textsForEmbed, u.embedBatchSize)
	if berr != nil {
		_ = u.ragRepo.MarkFileRAGIndexFailed(ctx, fileID, berr.Error())
		return fmt.Errorf("эмбеддинги: %w", berr)
	}
	embedDur := time.Since(embedPhase)

	dim := len(allVec[0])
	if dim == 0 {
		_ = u.ragRepo.MarkFileRAGIndexFailed(ctx, fileID, "пустой вектор эмбеддинга")
		return fmt.Errorf("эмбеддинги: нулевая размерность")
	}

	domainChunks := make([]domain.DocumentRAGChunk, len(rawChunks))
	for i := range rawChunks {
		if len(allVec[i]) != dim {
			_ = u.ragRepo.MarkFileRAGIndexFailed(ctx, fileID, "разная размерность векторов в батче")
			return fmt.Errorf("эмбеддинги: размерность %d vs %d", dim, len(allVec[i]))
		}
		h := sha256.Sum256([]byte(rawChunks[i].Text))
		meta := rawChunks[i].Metadata
		if meta == nil {
			meta = map[string]any{}
		}
		domainChunks[i] = domain.DocumentRAGChunk{
			ChatSessionID:       sessionID,
			UserID:              userID,
			FileID:              fileID,
			ChunkIndex:          i,
			Text:                rawChunks[i].Text,
			Metadata:            meta,
			ChunkContentSHA256:  hex.EncodeToString(h[:]),
			SourceContentSHA256: sourceHash,
			PipelineVersion:     domain.RAGPipelineVersion,
			EmbeddingModel:      embedModel,
			EmbeddingDim:        dim,
			Embedding:           allVec[i],
		}
	}

	storePhase := time.Now()
	if err := u.ragRepo.ReplaceFileChunks(ctx, sessionID, userID, fileID, domain.RAGPipelineVersion, embedModel, sourceHash, domainChunks); err != nil {
		_ = u.ragRepo.MarkFileRAGIndexFailed(ctx, fileID, err.Error())
		return err
	}
	storeDur := time.Since(storePhase)
	logger.I("DocumentIngest: готово file_id=%d session_id=%d chunks=%d model=%q extract=%s chunk=%s embed=%s store=%s total=%s",
		fileID, sessionID, len(domainChunks), embedModel,
		extractDur.Truncate(time.Millisecond), chunkDur.Truncate(time.Millisecond),
		embedDur.Truncate(time.Millisecond), storeDur.Truncate(time.Millisecond),
		time.Since(ingestStart).Truncate(time.Millisecond))
	return nil
}

func (u *DocumentIngestUseCase) SearchSessionKnowledge(ctx context.Context, userID int, sessionID int64, embeddingModel string, queryText string, topK int, restrictFileID *int64) ([]domain.ScoredDocumentRAGChunk, error) {
	session, err := u.sessionRepo.GetById(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if session.UserId != userID {
		return nil, domain.ErrUnauthorized
	}

	if restrictFileID != nil && *restrictFileID > 0 {
		if _, err := u.verifySessionFileOwnership(ctx, userID, sessionID, *restrictFileID); err != nil {
			return nil, err
		}
	}

	sessionModel := ""
	if session.SelectedRunnerID != nil {
		ru, rerr := u.runnerRepo.GetByID(ctx, *session.SelectedRunnerID)
		if rerr == nil && ru != nil {
			sessionModel = strings.TrimSpace(ru.SelectedModel)
		}
	}

	modelName, err := resolveModelForUser(ctx, u.llmRepo, strings.TrimSpace(embeddingModel), sessionModel)
	if err != nil {
		return nil, err
	}

	q := strings.TrimSpace(queryText)
	if q == "" {
		return nil, fmt.Errorf("пустой запрос")
	}

	vec, err := u.llmRepo.Embed(ctx, modelName, q)
	if err != nil {
		return nil, err
	}

	return u.ragRepo.SearchSessionTopK(ctx, sessionID, userID, modelName, vec, topK, restrictFileID)
}

func (u *DocumentIngestUseCase) verifySessionFileOwnership(ctx context.Context, userID int, sessionID, fileID int64) (*domain.File, error) {
	session, err := u.sessionRepo.GetById(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if session.UserId != userID {
		return nil, domain.ErrUnauthorized
	}

	f, err := u.fileRepo.GetById(ctx, fileID)
	if err != nil {
		return nil, err
	}

	if f == nil {
		return nil, fmt.Errorf("файл не найден")
	}

	if f.ChatSessionID == nil || *f.ChatSessionID != sessionID {
		return nil, fmt.Errorf("файл не относится к этой сессии")
	}

	if f.UserID == nil || *f.UserID != userID {
		return nil, fmt.Errorf("файл не принадлежит пользователю")
	}

	return f, nil
}

func (u *DocumentIngestUseCase) GetIngestionStatus(ctx context.Context, userID int, sessionID, fileID int64) (*domain.FileRAGIndex, error) {
	if _, err := u.verifySessionFileOwnership(ctx, userID, sessionID, fileID); err != nil {
		return nil, err
	}

	return u.ragRepo.GetFileIndex(ctx, fileID)
}

func (u *DocumentIngestUseCase) DeleteSessionFileIndex(ctx context.Context, userID int, sessionID, fileID int64) error {
	if _, err := u.verifySessionFileOwnership(ctx, userID, sessionID, fileID); err != nil {
		return err
	}

	return u.ragRepo.DeleteIndexForFile(ctx, fileID)
}
