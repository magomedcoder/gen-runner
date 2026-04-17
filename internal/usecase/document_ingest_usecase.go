package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/document"
	"github.com/magomedcoder/gen/pkg/logger"
	"github.com/magomedcoder/gen/pkg/rag"
)

const ragNeighborOnlyChunkScore = -1e9
const maxAdaptiveSearchTopK = 96

type DocumentIngestUseCase struct {
	sessionRepo                domain.ChatSessionRepository
	fileRepo                   domain.FileRepository
	ragRepo                    domain.DocumentRAGRepository
	runnerRepo                 domain.RunnerRepository
	llmRepo                    domain.LLMRepository
	attachmentsSaveDir         string
	splitOpts                  rag.SplitOptions
	embedBatchSize             int
	maxChunkEmbedRunes         int
	queryRewriteEnabled        bool
	queryRewriteMaxTokens      int32
	queryRewriteTimeoutSeconds int32
	hydeEnabled                bool
	hydeMaxTokens              int32
	hydeTimeoutSeconds         int32
	adaptiveKEnabled           bool
	adaptiveKMultiplier        int
	minSimilarityScore         float64
	rerankEnabled              bool
	rerankMaxCandidates        int
	rerankMaxTokens            int32
	rerankTimeoutSeconds       int32
	rerankPassageMaxRunes      int
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
	queryRewriteEnabled bool,
	queryRewriteMaxTokens int32,
	queryRewriteTimeoutSeconds int32,
	hydeEnabled bool,
	hydeMaxTokens int32,
	hydeTimeoutSeconds int32,
	adaptiveKEnabled bool,
	adaptiveKMultiplier int,
	minSimilarityScore float64,
	rerankEnabled bool,
	rerankMaxCandidates int,
	rerankMaxTokens int32,
	rerankTimeoutSeconds int32,
	rerankPassageMaxRunes int,
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
		sessionRepo:                sessionRepo,
		fileRepo:                   fileRepo,
		ragRepo:                    ragRepo,
		runnerRepo:                 runnerRepo,
		llmRepo:                    llmRepo,
		attachmentsSaveDir:         attachmentsSaveDir,
		splitOpts:                  splitOpts,
		embedBatchSize:             embedBatchSize,
		maxChunkEmbedRunes:         maxChunkEmbedRunes,
		queryRewriteEnabled:        queryRewriteEnabled,
		queryRewriteMaxTokens:      queryRewriteMaxTokens,
		queryRewriteTimeoutSeconds: queryRewriteTimeoutSeconds,
		hydeEnabled:                hydeEnabled,
		hydeMaxTokens:              hydeMaxTokens,
		hydeTimeoutSeconds:         hydeTimeoutSeconds,
		adaptiveKEnabled:           adaptiveKEnabled,
		adaptiveKMultiplier:        adaptiveKMultiplier,
		minSimilarityScore:         minSimilarityScore,
		rerankEnabled:              rerankEnabled,
		rerankMaxCandidates:        rerankMaxCandidates,
		rerankMaxTokens:            rerankMaxTokens,
		rerankTimeoutSeconds:       rerankTimeoutSeconds,
		rerankPassageMaxRunes:      rerankPassageMaxRunes,
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
	useTextCache := strings.EqualFold(f.ExtractedTextContentSha256, shaHex) && strings.TrimSpace(f.ExtractedText) != ""

	if useTextCache && strings.EqualFold(filepath.Ext(baseName), ".pdf") {
		useTextCache = false
	}

	var extracted string
	var pdfPageBounds []int
	if useTextCache {
		extracted = f.ExtractedText
	} else {
		extracted, pdfPageBounds, err = document.ExtractTextForRAG(baseName, data)
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

	var norm string
	if len(pdfPageBounds) >= 2 {
		norm = extracted
	} else {
		norm = document.NormalizeExtractedText(extracted)
	}

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
	var rawChunks []rag.Chunk
	if len(pdfPageBounds) >= 2 && strings.EqualFold(filepath.Ext(baseName), ".pdf") {
		rawChunks = rag.SplitTextWithPDFPageBounds(baseName, norm, u.splitOpts, pdfPageBounds)
	} else {
		rawChunks = rag.SplitText(baseName, norm, u.splitOpts)
	}

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

func (u *DocumentIngestUseCase) SearchSessionKnowledge(ctx context.Context, userID int, sessionID int64, embeddingModel string, queryText string, topK int, restrictFileID *int64, neighborChunkWindow int) ([]domain.ScoredDocumentRAGChunk, error) {
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
	if topK <= 0 {
		topK = 5
	}

	var rewriteMs int64
	var hydeMs int64
	qEmbed := q
	if u.queryRewriteEnabled {
		rewriteModel, rwErr := resolveModelForUser(ctx, u.llmRepo, "", sessionModel)
		if rwErr != nil {
			return nil, rwErr
		}

		tRw := time.Now()
		out, rwErr := u.rewriteQueryForRAGRetrieval(ctx, sessionID, q, rewriteModel)
		rewriteMs = time.Since(tRw).Milliseconds()
		if rwErr != nil {
			logger.W("DocumentIngest: переформулировка запроса RAG: %v - эмбеддинг без переформулировки", rwErr)
		} else {
			out = strings.TrimSpace(out)
			if out != "" && out != q {
				logger.I("DocumentIngest: переформулировка запроса RAG, мс=%d: %q -> %q", rewriteMs, q, out)
				qEmbed = out
			}
		}
	}

	if u.hydeEnabled {
		hydeModel, hyErr := resolveModelForUser(ctx, u.llmRepo, "", sessionModel)
		if hyErr != nil {
			return nil, hyErr
		}

		tHy := time.Now()
		pseudo, hyErr := u.generateHyDEPseudoDocument(ctx, sessionID, qEmbed, hydeModel)
		hydeMs = time.Since(tHy).Milliseconds()
		if hyErr != nil {
			logger.W("DocumentIngest: RAG HyDE: %v - эмбеддинг исходного запроса", hyErr)
		} else if pseudo != "" {
			qEmbed = pseudo
		}
	}

	tEmbed := time.Now()
	vec, err := u.llmRepo.Embed(ctx, modelName, qEmbed)
	embedMs := time.Since(tEmbed).Milliseconds()
	if err != nil {
		return nil, err
	}

	searchTopK := topK
	if u.adaptiveKEnabled && u.adaptiveKMultiplier > 1 {
		searchTopK = topK * u.adaptiveKMultiplier
		if searchTopK > maxAdaptiveSearchTopK {
			searchTopK = maxAdaptiveSearchTopK
		}
	}

	tSearch := time.Now()
	hits, err := u.ragRepo.SearchSessionTopK(ctx, sessionID, userID, modelName, vec, searchTopK, restrictFileID)
	searchMs := time.Since(tSearch).Milliseconds()
	if err != nil {
		return nil, err
	}

	vectorHitsRaw := len(hits)
	if u.minSimilarityScore > -1 {
		filtered := hits[:0]
		for _, h := range hits {
			if h.Score >= u.minSimilarityScore {
				filtered = append(filtered, h)
			}
		}
		hits = filtered
	}

	var rerankMs int64
	if u.rerankEnabled && len(hits) >= 2 {
		rerankModel, rerr := resolveModelForUser(ctx, u.llmRepo, "", sessionModel)
		if rerr != nil {
			logger.W("DocumentIngest: модель для переупорядочивания RAG: %v", rerr)
		} else {
			newHits, rms, rerr := u.rerankSearchHits(ctx, sessionID, q, hits, rerankModel)
			rerankMs = rms
			if rerr != nil {
				logger.W("DocumentIngest: переупорядочивание RAG: %v", rerr)
			} else {
				hits = newHits
			}
		}
	}

	if !u.adaptiveKEnabled && len(hits) > topK {
		hits = hits[:topK]
	}

	vectorHits := len(hits)
	var expandMs int64
	if neighborChunkWindow > 0 && len(hits) > 0 {
		tExp := time.Now()
		hits, err = u.expandSearchHitsWithNeighbors(ctx, sessionID, userID, modelName, restrictFileID, hits, neighborChunkWindow)
		expandMs = time.Since(tExp).Milliseconds()
		if err != nil {
			return nil, err
		}
	}

	logger.I("DocumentIngest: поиск по знаниям сессии сессия=%d topK=%d поиск_topK=%d адаптивный_k=%v порог_мин_оценки=%.3f совпадений_до_фильтра=%d совпадений=%d окно_соседей=%d чанков_всего=%d эмбеддинг_мс=%d поиск_мс=%d rerank_мс=%d расширение_мс=%d rewrite_мс=%d hyde_мс=%d", sessionID, topK, searchTopK, u.adaptiveKEnabled, u.minSimilarityScore, vectorHitsRaw, vectorHits, neighborChunkWindow, len(hits), embedMs, searchMs, rerankMs, expandMs, rewriteMs, hydeMs)

	return hits, nil
}

func (u *DocumentIngestUseCase) expandSearchHitsWithNeighbors(
	ctx context.Context,
	sessionID int64,
	userID int,
	embeddingModel string,
	restrictFileID *int64,
	hits []domain.ScoredDocumentRAGChunk,
	neighborWindow int,
) ([]domain.ScoredDocumentRAGChunk, error) {
	if neighborWindow <= 0 {
		return hits, nil
	}

	type pair struct {
		fid int64
		ix  int
	}

	primary := make(map[pair]float64)
	for _, h := range hits {
		k := pair{fid: h.FileID, ix: h.ChunkIndex}
		if s, ok := primary[k]; !ok || h.Score > s {
			primary[k] = h.Score
		}
	}

	byFile := make(map[int64]map[int]struct{})
	for k := range primary {
		if restrictFileID != nil && *restrictFileID > 0 && k.fid != *restrictFileID {
			continue
		}

		if byFile[k.fid] == nil {
			byFile[k.fid] = make(map[int]struct{})
		}

		for d := -neighborWindow; d <= neighborWindow; d++ {
			ix := k.ix + d
			if ix >= 0 {
				byFile[k.fid][ix] = struct{}{}
			}
		}
	}

	var out []domain.ScoredDocumentRAGChunk
	for fid, idxSet := range byFile {
		indices := make([]int, 0, len(idxSet))
		for ix := range idxSet {
			indices = append(indices, ix)
		}

		sort.Ints(indices)

		chunks, err := u.ragRepo.GetChunksByFileChunkIndices(ctx, sessionID, userID, fid, embeddingModel, indices)
		if err != nil {
			return nil, err
		}

		for _, ch := range chunks {
			k := pair{fid: ch.FileID, ix: ch.ChunkIndex}
			score := ragNeighborOnlyChunkScore
			if s, ok := primary[k]; ok {
				score = s
			}

			out = append(out, domain.ScoredDocumentRAGChunk{DocumentRAGChunk: ch, Score: score})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].FileID != out[j].FileID {
			return out[i].FileID < out[j].FileID
		}

		return out[i].ChunkIndex < out[j].ChunkIndex
	})

	return out, nil
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
