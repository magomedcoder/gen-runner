package domain

import "time"

type File struct {
	Id                         int64
	Filename                   string
	MimeType                   string
	Size                       int64
	StoragePath                string
	CreatedAt                  time.Time
	ChatSessionID              *int64
	UserID                     *int
	ExpiresAt                  *time.Time
	Kind                       string
	ExtractedText              string
	ExtractedTextContentSha256 string
}

func NewFile(filename, mimeType string, size int64, storagePath string) *File {
	return &File{
		Filename:    filename,
		MimeType:    mimeType,
		Size:        size,
		StoragePath: storagePath,
		CreatedAt:   time.Now(),
	}
}
