package handler

import (
	"context"
	"github.com/magomedcoder/gen/internal/delivery/mappers"

	"github.com/magomedcoder/gen/api/pb/authpb"
	"github.com/magomedcoder/gen/config"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthHandler struct {
	authpb.UnimplementedAuthServiceServer
	authUseCase *usecase.AuthUseCase
	cfg         *config.Config
}

func NewAuthHandler(cfg *config.Config, authUseCase *usecase.AuthUseCase) *AuthHandler {
	return &AuthHandler{
		cfg:         cfg,
		authUseCase: authUseCase,
	}
}

func (a *AuthHandler) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	logger.D("Login: username=%s", req.GetUsername())
	user, accessToken, refreshToken, err := a.authUseCase.Login(ctx, req.Username, req.Password)
	if err != nil {
		logger.W("Login: %v", err)
		return nil, ToStatusError(codes.Unauthenticated, err)
	}
	logger.I("Login: успешный вход user=%d", user.Id)

	return &authpb.LoginResponse{
		User:         mappers.UserToProto(user),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (a *AuthHandler) RefreshToken(ctx context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	logger.D("RefreshToken: запрос")
	accessToken, refreshToken, err := a.authUseCase.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		logger.W("RefreshToken: %v", err)
		return nil, ToStatusError(codes.Unauthenticated, err)
	}
	logger.I("RefreshToken: успешно")

	return &authpb.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (a *AuthHandler) Logout(ctx context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
	user, err := GetUserFromContext(ctx, a.authUseCase)
	if err != nil {
		return nil, err
	}

	if err := a.authUseCase.Logout(ctx, user.Id); err != nil {
		logger.E("Logout: %v", err)
		return nil, status.Error(codes.Internal, "не удалось выйти из системы")
	}
	logger.I("Logout: user=%d вышел", user.Id)

	return &authpb.LogoutResponse{
		Success: true,
	}, nil
}

func (a *AuthHandler) ChangePassword(ctx context.Context, req *authpb.ChangePasswordRequest) (*authpb.ChangePasswordResponse, error) {
	user, err := GetUserFromContext(ctx, a.authUseCase)
	if err != nil {
		return nil, err
	}

	if err := a.authUseCase.ChangePassword(ctx, user.Id, req.OldPassword, req.NewPassword); err != nil {
		logger.W("ChangePassword: %v", err)
		return nil, ToStatusError(codes.InvalidArgument, err)
	}
	logger.I("ChangePassword: пароль изменён user=%d", user.Id)

	return &authpb.ChangePasswordResponse{
		Success: true,
	}, nil
}

func (a *AuthHandler) CheckVersion(ctx context.Context, req *authpb.CheckVersionRequest) (*authpb.CheckVersionResponse, error) {
	clientBuild := req.GetClientBuild()
	compatible := clientBuild >= a.cfg.MinClientBuild

	msg := ""
	if !compatible {
		msg = "Версия приложения несовместима с сервером"
	}

	return &authpb.CheckVersionResponse{
		Compatible: compatible,
		Message:    msg,
	}, nil
}
