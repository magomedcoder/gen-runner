package handler

import (
	"errors"

	"github.com/magomedcoder/gen/internal/domain"
	"google.golang.org/grpc/codes"
)

func statusForChatAssistantOpSentinel(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, domain.ErrRegenerateToolsNotSupported):
		return StatusErrorWithReason(codes.FailedPrecondition, "CHAT_REGENERATE_TOOLS_NOT_SUPPORTED", err.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		return StatusErrorWithReason(codes.PermissionDenied, "CHAT_UNAUTHORIZED", err.Error())
	default:
		return nil
	}
}

func statusForCreateSessionError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, domain.ErrNoRunners):
		return StatusErrorWithReason(codes.FailedPrecondition, "CHAT_NO_RUNNERS", err.Error())
	case errors.Is(err, domain.ErrRunnerChatModelNotConfigured):
		return StatusErrorWithReason(codes.FailedPrecondition, "CHAT_RUNNER_MODEL_NOT_CONFIGURED", err.Error())
	default:
		return nil
	}
}
