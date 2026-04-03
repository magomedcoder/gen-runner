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
	"testing"

	"github.com/magomedcoder/gen/internal/domain"
)

func TestNormalizeAttachmentHydrateParallelism(t *testing.T) {
	if normalizeAttachmentHydrateParallelism(0) != 8 {
		t.Fatal("0 -> 8")
	}

	if normalizeAttachmentHydrateParallelism(3) != 3 {
		t.Fatal("3")
	}

	if normalizeAttachmentHydrateParallelism(100) != 64 {
		t.Fatal("cap 64")
	}
}

type mapFileRepo map[int64]*domain.File

func (m mapFileRepo) Create(context.Context, *domain.File) error {
	return nil
}

func (m mapFileRepo) UpdateStoragePath(context.Context, int64, string) error {
	return nil
}

func (m mapFileRepo) GetById(_ context.Context, id int64) (*domain.File, error) {
	f, ok := m[id]
	if !ok {
		return nil, fmt.Errorf("файл в репозитории: запись с id=%d не найдена", id)
	}

	return f, nil
}

func (m mapFileRepo) ListByIds(_ context.Context, ids []int64) ([]*domain.File, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	seen := make(map[int64]struct{}, len(ids))
	out := make([]*domain.File, 0, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}

		seen[id] = struct{}{}
		f, err := m.GetById(context.Background(), id)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}

	return out, nil
}

func (m mapFileRepo) CountSessionTTLArtifacts(context.Context, int64, int) (int32, int64, error) {
	return 0, 0, nil
}

func (m mapFileRepo) SaveExtractedTextCache(context.Context, int64, string, string) error {
	return nil
}

type spyFileRepo struct {
	files    map[int64]*domain.File
	saveHits int
}

func (s *spyFileRepo) Create(context.Context, *domain.File) error {
	return nil
}

func (s *spyFileRepo) UpdateStoragePath(context.Context, int64, string) error {
	return nil
}

func (s *spyFileRepo) GetById(_ context.Context, id int64) (*domain.File, error) {
	f, ok := s.files[id]
	if !ok {
		return nil, fmt.Errorf("файл id=%d не найден", id)
	}

	return f, nil
}

func (s *spyFileRepo) ListByIds(_ context.Context, ids []int64) ([]*domain.File, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	seen := make(map[int64]struct{}, len(ids))
	out := make([]*domain.File, 0, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}

		seen[id] = struct{}{}
		f, err := s.GetById(context.Background(), id)
		if err != nil {
			return nil, err
		}

		out = append(out, f)
	}

	return out, nil
}

func (s *spyFileRepo) SaveExtractedTextCache(_ context.Context, fileID int64, sha256Hex, extractedText string) error {
	s.saveHits++
	f := s.files[fileID]
	if f != nil {
		f.ExtractedText = extractedText
		f.ExtractedTextContentSha256 = sha256Hex
	}

	return nil
}

func (s *spyFileRepo) CountSessionTTLArtifacts(context.Context, int64, int) (int32, int64, error) {
	return 0, 0, nil
}

func TestHydrateAttachmentsForRunner_loadsImageFromDisk(t *testing.T) {
	dir := t.TempDir()
	sid := int64(7)
	sessDir := filepath.Join(dir, strconv.FormatInt(sid, 10))
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(sessDir, "42_img.png")
	want := []byte{0x89, 0x50, 0x4e, 0x47, 0x01, 0x02}
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}

	fid := int64(42)
	c := &ChatUseCase{
		fileRepo: mapFileRepo{
			fid: {
				Id:          fid,
				Filename:    "img.png",
				StoragePath: path,
			},
		},
		attachmentsSaveDir: dir,
	}

	msgs := []*domain.Message{
		{
			SessionId:        sid,
			AttachmentName:   "img.png",
			AttachmentFileID: &fid,
			Content:          "подпись",
		},
	}

	if err := c.hydrateAttachmentsForRunner(context.Background(), msgs); err != nil {
		t.Fatal(err)
	}

	if string(msgs[0].AttachmentContent) != string(want) {
		t.Fatalf("содержимое вложения не совпадает с файлом на диске")
	}
}

func TestHydrateAttachmentsForRunner_rejectsPathOutsideSession(t *testing.T) {
	dir := t.TempDir()
	sid := int64(7)
	other := filepath.Join(dir, "other", "evil.png")
	_ = os.MkdirAll(filepath.Dir(other), 0o755)
	_ = os.WriteFile(other, []byte{1, 2, 3}, 0o644)

	fid := int64(99)
	c := &ChatUseCase{
		fileRepo: mapFileRepo{
			fid: {
				Id:          fid,
				Filename:    "x.png",
				StoragePath: other,
			},
		},
		attachmentsSaveDir: dir,
	}
	msgs := []*domain.Message{
		{
			SessionId:        sid,
			AttachmentName:   "x.png",
			AttachmentFileID: &fid,
		},
	}
	if err := c.hydrateAttachmentsForRunner(context.Background(), msgs); err == nil {
		t.Fatal("ожидалась ошибка: путь к файлу вне каталога сессии")
	}
}

func TestHydrateAttachmentsForRunner_expandsDocumentText(t *testing.T) {
	dir := t.TempDir()
	sid := int64(7)
	sessDir := filepath.Join(dir, strconv.FormatInt(sid, 10))
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(sessDir, "55_notes.txt")
	want := []byte("альфа")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}

	fid := int64(55)
	c := &ChatUseCase{
		fileRepo: mapFileRepo{
			fid: {
				Id:          fid,
				Filename:    "notes.txt",
				StoragePath: path,
			},
		},
		attachmentsSaveDir: dir,
	}

	msgs := []*domain.Message{
		{
			SessionId:        sid,
			AttachmentName:   "notes.txt",
			AttachmentFileID: &fid,
			Content:          "вопрос",
		},
	}

	if err := c.hydrateAttachmentsForRunner(context.Background(), msgs); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(msgs[0].Content, "альфа") || !strings.Contains(msgs[0].Content, "вопрос") {
		t.Fatalf("ожидалось развёрнутое содержимое, получено %q", msgs[0].Content)
	}
}

func TestHydrateAttachmentsForRunner_secondRoundUsesExtractedTextCache(t *testing.T) {
	dir := t.TempDir()
	sid := int64(7)
	sessDir := filepath.Join(dir, strconv.FormatInt(sid, 10))
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(sessDir, "55_notes.txt")
	raw := []byte("альфа")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	fid := int64(55)
	fcopy := &domain.File{
		Id:          fid,
		Filename:    "notes.txt",
		StoragePath: path,
	}
	spy := &spyFileRepo{files: map[int64]*domain.File{fid: fcopy}}
	c := &ChatUseCase{
		fileRepo:           spy,
		attachmentsSaveDir: dir,
	}

	msg := &domain.Message{
		SessionId:        sid,
		AttachmentName:   "notes.txt",
		AttachmentFileID: &fid,
		Content:          "вопрос",
	}

	if err := c.hydrateAttachmentsForRunner(context.Background(), []*domain.Message{msg}); err != nil {
		t.Fatal(err)
	}

	if spy.saveHits != 1 {
		t.Fatalf("ожидали одно сохранение кэша, получено %d", spy.saveHits)
	}

	msg2 := &domain.Message{
		SessionId:        sid,
		AttachmentName:   "notes.txt",
		AttachmentFileID: &fid,
		Content:          "второй",
	}

	if err := c.hydrateAttachmentsForRunner(context.Background(), []*domain.Message{msg2}); err != nil {
		t.Fatal(err)
	}

	if spy.saveHits != 1 {
		t.Fatalf("повторная гидратация не должна вызывать SaveExtractedTextCache, saveHits=%d", spy.saveHits)
	}

	if !strings.Contains(msg2.Content, "альфа") || !strings.Contains(msg2.Content, "второй") {
		t.Fatalf("ожидался кэшированный текст файла, получено %q", msg2.Content)
	}
}

func TestHydrateAttachmentsForRunner_prefersShaMatchedExtractedCache(t *testing.T) {
	dir := t.TempDir()
	sid := int64(7)
	sessDir := filepath.Join(dir, strconv.FormatInt(sid, 10))
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(sessDir, "55_notes.txt")
	diskBody := []byte("на-диске")
	if err := os.WriteFile(path, diskBody, 0o644); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(diskBody)
	shaHex := hex.EncodeToString(sum[:])

	fid := int64(55)
	c := &ChatUseCase{
		fileRepo: mapFileRepo{
			fid: {
				Id:                         fid,
				Filename:                   "notes.txt",
				StoragePath:                path,
				ExtractedText:              "только-из-кэша",
				ExtractedTextContentSha256: shaHex,
			},
		},
		attachmentsSaveDir: dir,
	}

	msg := &domain.Message{
		SessionId:        sid,
		AttachmentName:   "notes.txt",
		AttachmentFileID: &fid,
		Content:          "вопрос",
	}
	if err := c.hydrateAttachmentsForRunner(context.Background(), []*domain.Message{msg}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(msg.Content, "только-из-кэша") {
		t.Fatalf("ожидался текст из кэша БД, получено %q", msg.Content)
	}

	if strings.Contains(msg.Content, "на-диске") {
		t.Fatal("не должны подставлять извлечение с диска при совпадении sha256")
	}
}
