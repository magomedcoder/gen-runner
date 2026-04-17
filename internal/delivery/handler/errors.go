package handler

import (
	"errors"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc/codes"
)

func ToStatusError(code codes.Code, err error) error {
	msg := safeMessage(code)
	if code == codes.Internal && err != nil {
		logger.E("handler: внутренняя ошибка: %v", err)
	}

	return StatusErrorWithReason(code, genericReasonForCode(code), msg)
}

func genericReasonForCode(code codes.Code) string {
	switch code {
	case codes.Internal:
		return "GEN_INTERNAL_ERROR"
	case codes.NotFound:
		return "GEN_NOT_FOUND"
	case codes.InvalidArgument:
		return "GEN_INVALID_ARGUMENT"
	case codes.FailedPrecondition:
		return "GEN_FAILED_PRECONDITION"
	case codes.PermissionDenied:
		return "GEN_PERMISSION_DENIED"
	case codes.Unavailable:
		return "GEN_UNAVAILABLE"
	case codes.Unauthenticated:
		return "GEN_UNAUTHENTICATED"
	case codes.ResourceExhausted:
		return "GEN_RESOURCE_EXHAUSTED"
	default:
		return "GEN_ERROR"
	}
}

func safeMessage(code codes.Code) string {
	switch code {
	case codes.Internal:
		return "внутренняя ошибка сервера"
	case codes.Unauthenticated:
		return "неверные учётные данные"
	case codes.NotFound:
		return "не найдено"
	case codes.InvalidArgument:
		return "неверный запрос"
	case codes.FailedPrecondition:
		return "сервис не готов к выполнению запроса"
	case codes.PermissionDenied:
		return "доступ запрещён"
	case codes.Unavailable:
		return "сервис временно недоступен"
	default:
		return "произошла ошибка"
	}
}

func statusForModelResolutionError(err error) error {
	if err == nil {
		return nil
	}

	msg := err.Error()
	if strings.Contains(msg, "модель") && strings.Contains(msg, "недоступна") {
		return StatusErrorWithReason(codes.InvalidArgument, "CHAT_MODEL_UNAVAILABLE", msg)
	}

	if strings.Contains(msg, "нет доступных моделей") {
		return StatusErrorWithReason(codes.FailedPrecondition, "CHAT_NO_MODELS_AVAILABLE", msg)
	}

	return nil
}

func statusForSessionScopedGetError(err error) error {
	msg := err.Error()
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		return StatusErrorWithReason(codes.PermissionDenied, "CHAT_UNAUTHORIZED", "нет доступа к сессии")
	case strings.Contains(msg, "некорректный"),
		strings.Contains(msg, "не найдено"):
		return StatusErrorWithReason(codes.InvalidArgument, "CHAT_SESSION_READ_INVALID_ARGUMENT", msg)
	default:
		return ToStatusError(codes.Internal, err)
	}
}
