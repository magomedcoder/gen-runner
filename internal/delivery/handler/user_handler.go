package handler

import (
	"context"
	"github.com/magomedcoder/gen/internal/delivery/mappers"
	"strconv"

	"github.com/magomedcoder/gen/api/pb/userpb"
	"github.com/magomedcoder/gen/internal/usecase"
	"github.com/magomedcoder/gen/pkg/logger"
	"google.golang.org/grpc/codes"
)

type UserHandler struct {
	userpb.UnimplementedUserServiceServer
	userUseCase *usecase.UserUseCase
	authUseCase *usecase.AuthUseCase
}

func NewUserHandler(userUseCase *usecase.UserUseCase, authUseCase *usecase.AuthUseCase) *UserHandler {
	return &UserHandler{
		userUseCase: userUseCase,
		authUseCase: authUseCase,
	}
}

func (u *UserHandler) GetUsers(ctx context.Context, req *userpb.GetUsersRequest) (*userpb.GetUsersResponse, error) {
	logger.D("GetUsers: запрос списка пользователей")
	if err := RequireAdmin(ctx, u.authUseCase); err != nil {
		return nil, err
	}

	users, total, err := u.userUseCase.GetUsers(ctx, req.Page, req.PageSize)
	if err != nil {
		logger.E("GetUsers: %v", err)
		return nil, ToStatusError(codes.Internal, err)
	}
	logger.I("GetUsers: возвращено пользователей: %d, всего: %d", len(users), total)

	resp := &userpb.GetUsersResponse{
		Total: total,
	}
	for _, user := range users {
		resp.Users = append(resp.Users, mappers.UserToProto(user))
	}

	return resp, nil
}

func (u *UserHandler) CreateUser(ctx context.Context, req *userpb.CreateUserRequest) (*userpb.CreateUserResponse, error) {
	logger.D("CreateUser: username=%s", req.GetUsername())
	if err := RequireAdmin(ctx, u.authUseCase); err != nil {
		return nil, err
	}

	user, err := u.userUseCase.CreateUser(ctx, req.Username, req.Password, req.Name, req.Surname, req.Role)
	if err != nil {
		logger.W("CreateUser: %v", err)
		return nil, ToStatusError(codes.InvalidArgument, err)
	}
	logger.I("CreateUser: пользователь создан id=%d", user.Id)
	return &userpb.CreateUserResponse{
		User: mappers.UserToProto(user),
	}, nil
}

func (u *UserHandler) EditUser(ctx context.Context, req *userpb.EditUserRequest) (*userpb.EditUserResponse, error) {
	logger.D("EditUser: id=%s", req.GetId())
	if err := RequireAdmin(ctx, u.authUseCase); err != nil {
		return nil, err
	}

	if _, err := strconv.Atoi(req.Id); err != nil {
		logger.W("EditUser: неверный id: %s", req.Id)
		return nil, StatusErrorWithReason(codes.InvalidArgument, "USER_INVALID_ID", "неверный id пользователя")
	}

	user, err := u.userUseCase.EditUser(ctx, req.Id, req.Username, req.Password, req.Name, req.Surname, req.Role)
	if err != nil {
		logger.W("EditUser: %v", err)
		return nil, ToStatusError(codes.InvalidArgument, err)
	}
	logger.I("EditUser: пользователь обновлён id=%s", req.Id)
	return &userpb.EditUserResponse{
		User: mappers.UserToProto(user),
	}, nil
}
