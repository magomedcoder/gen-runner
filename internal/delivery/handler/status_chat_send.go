package handler

import (
	"errors"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/document"
	"google.golang.org/grpc/codes"
)

func statusForDocumentAttachmentError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, document.ErrUnsupportedAttachmentType):
		return StatusErrorWithReason(codes.InvalidArgument, "CHAT_ATTACHMENT_UNSUPPORTED_TYPE", err.Error())
	case errors.Is(err, document.ErrInvalidTextEncoding):
		return StatusErrorWithReason(codes.InvalidArgument, "CHAT_ATTACHMENT_INVALID_ENCODING", err.Error())
	case errors.Is(err, document.ErrTextExtractionFailed):
		return StatusErrorWithReason(codes.InvalidArgument, "CHAT_ATTACHMENT_TEXT_EXTRACT_FAILED", err.Error())
	case errors.Is(err, document.ErrNoExtractableText):
		return StatusErrorWithReason(codes.InvalidArgument, "CHAT_ATTACHMENT_NO_EXTRACTABLE_TEXT", err.Error())
	default:
		return nil
	}
}

func statusForChatSendError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, domain.ErrRAGNotConfigured):
		return StatusErrorWithReason(codes.FailedPrecondition, "CHAT_RAG_NOT_CONFIGURED", err.Error())
	case errors.Is(err, domain.ErrRAGIndexNotReady):
		return StatusErrorWithReason(codes.FailedPrecondition, "CHAT_RAG_INDEX_NOT_READY", err.Error())
	case errors.Is(err, domain.ErrRAGIndexFailed):
		return StatusErrorWithReason(codes.FailedPrecondition, "CHAT_RAG_INDEX_FAILED", err.Error())
	case errors.Is(err, domain.ErrRAGNoHits):
		return StatusErrorWithReason(codes.FailedPrecondition, "CHAT_RAG_NO_HITS", err.Error())
	default:
		return statusForDocumentAttachmentError(err)
	}
}
