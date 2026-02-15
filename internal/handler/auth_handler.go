package handler

import (
	"context"

	"github.com/magomedcoder/gen/api/pb"
	"github.com/magomedcoder/gen/internal/mappers"
	"github.com/magomedcoder/gen/internal/usecase"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthHandler struct {
	pb.UnimplementedAuthServiceServer
	authUseCase *usecase.AuthUseCase
}

func NewAuthHandler(authUseCase *usecase.AuthUseCase) *AuthHandler {
	return &AuthHandler{
		authUseCase: authUseCase,
	}
}

func (a *AuthHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	user, accessToken, refreshToken, err := a.authUseCase.Login(ctx, req.Username, req.Password)
	if err != nil {
		return nil, ToStatusError(codes.Unauthenticated, err)
	}

	return &pb.LoginResponse{
		User:         mappers.UserToProto(user),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (a *AuthHandler) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	accessToken, refreshToken, err := a.authUseCase.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		return nil, ToStatusError(codes.Unauthenticated, err)
	}

	return &pb.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (a *AuthHandler) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	user, err := getUserFromContext(ctx, a.authUseCase)
	if err != nil {
		return nil, err
	}

	if err := a.authUseCase.Logout(ctx, user.Id); err != nil {
		return nil, status.Error(codes.Internal, "не удалось выйти из системы")
	}

	return &pb.LogoutResponse{
		Success: true,
	}, nil
}

func (a *AuthHandler) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) (*pb.ChangePasswordResponse, error) {
	user, err := getUserFromContext(ctx, a.authUseCase)
	if err != nil {
		return nil, err
	}

	if err := a.authUseCase.ChangePassword(ctx, user.Id, req.OldPassword, req.NewPassword); err != nil {
		return nil, ToStatusError(codes.InvalidArgument, err)
	}

	return &pb.ChangePasswordResponse{Success: true}, nil
}
