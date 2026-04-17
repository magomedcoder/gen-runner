package handler

import (
	"context"
	"strings"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/usecase"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

func extractToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", StatusErrorWithReason(codes.Unauthenticated, "AUTH_METADATA_MISSING", "метаданные не предоставлены")
	}

	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return "", StatusErrorWithReason(codes.Unauthenticated, "AUTH_AUTHORIZATION_HEADER_MISSING", "заголовок авторизации не предоставлен")
	}

	authHeader := authHeaders[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", StatusErrorWithReason(codes.Unauthenticated, "AUTH_BEARER_FORMAT_INVALID", "неверный формат заголовка авторизации")
	}

	return strings.TrimPrefix(authHeader, "Bearer "), nil
}

func GetUserFromContext(ctx context.Context, authUseCase *usecase.AuthUseCase) (*domain.User, error) {
	token, err := extractToken(ctx)
	if err != nil {
		return nil, err
	}

	user, err := authUseCase.ValidateToken(ctx, token)
	if err != nil {
		return nil, StatusErrorWithReason(codes.Unauthenticated, "AUTH_TOKEN_INVALID", err.Error())
	}

	return user, nil
}

func RequireAdmin(ctx context.Context, authUseCase *usecase.AuthUseCase) error {
	user, err := GetUserFromContext(ctx, authUseCase)
	if err != nil {
		return err
	}

	if user.Role != domain.UserRoleAdmin {
		return StatusErrorWithReason(codes.PermissionDenied, "AUTH_ADMIN_REQUIRED", "доступ разрешён только администратору")
	}

	return nil
}
