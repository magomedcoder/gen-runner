package handler

import (
	"context"
	"strconv"

	"github.com/magomedcoder/gen/api/pb"
	"github.com/magomedcoder/gen/internal/mappers"
	"github.com/magomedcoder/gen/internal/usecase"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserHandler struct {
	pb.UnimplementedUserServiceServer
	userUseCase *usecase.UserUseCase
	authUseCase *usecase.AuthUseCase
}

func NewUserHandler(userUseCase *usecase.UserUseCase, authUseCase *usecase.AuthUseCase) *UserHandler {
	return &UserHandler{
		userUseCase: userUseCase,
		authUseCase: authUseCase,
	}
}

func (u *UserHandler) GetUsers(ctx context.Context, req *pb.GetUsersRequest) (*pb.GetUsersResponse, error) {
	if err := requireAdmin(ctx, u.authUseCase); err != nil {
		return nil, err
	}

	users, total, err := u.userUseCase.GetUsers(ctx, req.Page, req.PageSize)
	if err != nil {
		return nil, ToStatusError(codes.Internal, err)
	}

	resp := &pb.GetUsersResponse{
		Total: total,
	}
	for _, user := range users {
		resp.Users = append(resp.Users, mappers.UserToProto(user))
	}

	return resp, nil
}

func (u *UserHandler) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	if err := requireAdmin(ctx, u.authUseCase); err != nil {
		return nil, err
	}

	user, err := u.userUseCase.CreateUser(ctx, req.Username, req.Password, req.Name, req.Surname, req.Role)
	if err != nil {
		return nil, ToStatusError(codes.InvalidArgument, err)
	}

	return &pb.CreateUserResponse{User: mappers.UserToProto(user)}, nil
}

func (u *UserHandler) EditUser(ctx context.Context, req *pb.EditUserRequest) (*pb.EditUserResponse, error) {
	if err := requireAdmin(ctx, u.authUseCase); err != nil {
		return nil, err
	}

	if _, err := strconv.Atoi(req.Id); err != nil {
		return nil, status.Error(codes.InvalidArgument, "неверный id пользователя")
	}

	user, err := u.userUseCase.EditUser(ctx, req.Id, req.Username, req.Password, req.Name, req.Surname, req.Role)
	if err != nil {
		return nil, ToStatusError(codes.InvalidArgument, err)
	}

	return &pb.EditUserResponse{User: mappers.UserToProto(user)}, nil
}
