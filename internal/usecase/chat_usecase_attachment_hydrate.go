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
	"golang.org/x/sync/errgroup"
)

const maxFileExtractedTextCacheBytes = 2 << 20

func (c *ChatUseCase) hydrateAttachmentsForRunner(ctx context.Context, messages []*domain.Message) error {
	if len(messages) == 0 {
		return nil
	}

	if strings.TrimSpace(c.attachmentsSaveDir) == "" {
		for _, m := range messages {
			if m != nil && m.AttachmentFileID != nil && len(m.AttachmentContent) == 0 {
				return fmt.Errorf("вложение в истории чата (file_id=%d): не задан каталог вложений", *m.AttachmentFileID)
			}
		}

		return nil
	}

	needIDs := make(map[int64]struct{})
	for _, m := range messages {
		if m == nil || m.AttachmentFileID == nil || len(m.AttachmentContent) > 0 {
			continue
		}

		needIDs[*m.AttachmentFileID] = struct{}{}
	}

	if len(needIDs) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(needIDs))
	for id := range needIDs {
		ids = append(ids, id)
	}

	files, err := c.fileRepo.ListByIds(ctx, ids)
	if err != nil {
		return fmt.Errorf("пакетная загрузка файлов вложений: %w", err)
	}

	byID := make(map[int64]*domain.File, len(files))
	for _, f := range files {
		if f != nil {
			byID[f.Id] = f
		}
	}

	for id := range needIDs {
		if _, ok := byID[id]; !ok {
			return fmt.Errorf("файл вложения id=%d не найден", id)
		}
	}

	type hydrateTask struct {
		msg             *domain.Message
		includeUserText bool
	}
	var toHydrate []hydrateTask
	for i, m := range messages {
		if m == nil || m.AttachmentFileID == nil || len(m.AttachmentContent) > 0 {
			continue
		}

		toHydrate = append(toHydrate, hydrateTask{
			msg:             m,
			includeUserText: i == len(messages)-1,
		})
	}

	if len(toHydrate) == 0 {
		return nil
	}

	sem := make(chan struct{}, normalizeAttachmentHydrateParallelism(c.attachmentHydrateParallelism))
	g, gctx := errgroup.WithContext(ctx)
	for _, task := range toHydrate {
		m := task.msg
		includeUserText := task.includeUserText
		f := byID[*m.AttachmentFileID]
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			return c.hydrateOneAttachmentForRunner(gctx, m, f, includeUserText)
		})
	}

	return g.Wait()
}

func (c *ChatUseCase) hydrateOneAttachmentForRunner(ctx context.Context, m *domain.Message, f *domain.File, includeUserText bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if f.ExpiresAt != nil && time.Now().After(*f.ExpiresAt) {
		return fmt.Errorf("файл вложения id=%d: истёк срок хранения", *m.AttachmentFileID)
	}

	path := strings.TrimSpace(f.StoragePath)
	if path == "" {
		return fmt.Errorf("файл вложения id=%d: пустой storage_path", *m.AttachmentFileID)
	}

	expectedDir := filepath.Clean(filepath.Join(c.attachmentsSaveDir, strconv.FormatInt(m.SessionId, 10)))
	cleanPath := filepath.Clean(path)
	if !strings.HasPrefix(cleanPath, expectedDir+string(filepath.Separator)) && cleanPath != expectedDir {
		return fmt.Errorf("файл вложения id=%d: путь вне каталога сессии", *m.AttachmentFileID)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("чтение вложения %q: %w", path, err)
	}

	if len(data) > document.MaxRecommendedAttachmentSizeBytes {
		return fmt.Errorf("вложение %q превышает лимит %d байт", path, document.MaxRecommendedAttachmentSizeBytes)
	}

	name := strings.TrimSpace(m.AttachmentName)
	if name == "" {
		name = filepath.Base(f.Filename)
	}

	if document.IsImageAttachment(name) {
		if err := document.ValidateImageAttachment(name, data); err != nil {
			return err
		}

		m.AttachmentContent = data

		return nil
	}

	if err := document.ValidateAttachment(name, data); err != nil {
		return err
	}

	sum := sha256.Sum256(data)
	shaHex := hex.EncodeToString(sum[:])

	var built string
	if strings.EqualFold(f.ExtractedTextContentSha256, shaHex) && strings.TrimSpace(f.ExtractedText) != "" {
		built = buildCompactHydratedAttachmentMessage(name, f.ExtractedText, m.Content, includeUserText)
	} else {
		extracted, err := document.ExtractText(name, data)
		if err != nil {
			logger.W("ChatUseCase: извлечение текста из вложения %q: %v", name, err)
			return fmt.Errorf("%w: %v", document.ErrTextExtractionFailed, err)
		}

		built = buildCompactHydratedAttachmentMessage(name, extracted, m.Content, includeUserText)

		if len(extracted) <= maxFileExtractedTextCacheBytes {
			if err := c.fileRepo.SaveExtractedTextCache(ctx, f.Id, shaHex, extracted); err != nil {
				logger.W("ChatUseCase: кэш извлечённого текста file_id=%d: %v", f.Id, err)
			}
		}
	}

	m.Content = built
	return nil
}

func (c *ChatUseCase) loadSessionAttachmentForSend(ctx context.Context, userID int, sessionID int64, fileID int64) (name string, content []byte, err error) {
	if strings.TrimSpace(c.attachmentsSaveDir) == "" {
		return "", nil, fmt.Errorf("хранилище вложений не настроено")
	}

	f, err := c.fileRepo.GetById(ctx, fileID)
	if err != nil {
		return "", nil, fmt.Errorf("файл id=%d: %w", fileID, err)
	}

	if f == nil {
		return "", nil, fmt.Errorf("файл id=%d не найден", fileID)
	}

	if f.ChatSessionID == nil || *f.ChatSessionID != sessionID {
		return "", nil, fmt.Errorf("файл не относится к этой сессии")
	}

	if f.UserID == nil || *f.UserID != userID {
		return "", nil, fmt.Errorf("файл не принадлежит пользователю")
	}

	if f.ExpiresAt != nil && time.Now().After(*f.ExpiresAt) {
		return "", nil, fmt.Errorf("срок действия файла истёк")
	}

	path := strings.TrimSpace(f.StoragePath)
	if path == "" {
		return "", nil, fmt.Errorf("файл id=%d: пустой storage_path", fileID)
	}

	expectedDir := filepath.Clean(filepath.Join(c.attachmentsSaveDir, strconv.FormatInt(sessionID, 10)))
	cleanPath := filepath.Clean(path)
	if !strings.HasPrefix(cleanPath, expectedDir+string(filepath.Separator)) && cleanPath != expectedDir {
		return "", nil, fmt.Errorf("файл id=%d: неверный путь хранения", fileID)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("чтение файла: %w", err)
	}

	if len(data) > document.MaxRecommendedAttachmentSizeBytes {
		return "", nil, fmt.Errorf("размер вложения превышает рекомендуемый максимум: %d байт", document.MaxRecommendedAttachmentSizeBytes)
	}

	baseName := filepath.Base(f.Filename)
	if baseName == "" || baseName == "." {
		baseName = "file"
	}

	if document.IsImageAttachment(baseName) {
		if err := document.ValidateImageAttachment(baseName, data); err != nil {
			return "", nil, err
		}
	} else if err := document.ValidateAttachment(baseName, data); err != nil {
		return "", nil, err
	}

	return baseName, data, nil
}
