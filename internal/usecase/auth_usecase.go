package usecase

import (
	"context"
	"errors"

	"github.com/magomedcoder/gen/internal/domain"
	"github.com/magomedcoder/gen/internal/service"
	"github.com/magomedcoder/gen/pkg"
	"github.com/magomedcoder/gen/pkg/logger"
)

type AuthUseCase struct {
	authTx     domain.AuthTransactionRunner
	userRepo   domain.UserRepository
	tokenRepo  domain.TokenRepository
	jwtService *service.JWTService
}

func NewAuthUseCase(
	authTx domain.AuthTransactionRunner,
	userRepo domain.UserRepository,
	tokenRepo domain.TokenRepository,
	jwtService *service.JWTService,
) *AuthUseCase {
	return &AuthUseCase{
		authTx:     authTx,
		userRepo:   userRepo,
		tokenRepo:  tokenRepo,
		jwtService: jwtService,
	}
}

func (a *AuthUseCase) Login(ctx context.Context, username, password string) (*domain.User, string, string, error) {
	logger.D("Login: username=%s", username)
	user, err := a.userRepo.GetByUsername(ctx, username)
	if err != nil {
		logger.W("Login: пользователь не найден: %s", username)
		return nil, "", "", errors.New("неверные учетные данные")
	}

	if !a.jwtService.CheckPassword(user.Password, password) {
		logger.W("Login: неверный пароль: %s", username)
		return nil, "", "", errors.New("неверные учетные данные")
	}

	accessToken, accessExpires, err := a.jwtService.GenerateAccessToken(user)
	if err != nil {
		logger.E("Login: генерация access token: %v", err)
		return nil, "", "", err
	}

	refreshToken, refreshExpires, err := a.jwtService.GenerateRefreshToken(user)
	if err != nil {
		return nil, "", "", err
	}

	accessTokenEntity := domain.NewToken(user.Id, accessToken, domain.TokenTypeAccess, accessExpires)
	refreshTokenEntity := domain.NewToken(user.Id, refreshToken, domain.TokenTypeRefresh, refreshExpires)

	if err := a.authTx.WithinTx(ctx, func(ctx context.Context, r domain.AuthRepos) error {
		_ = r.Token.DeleteByUserId(ctx, user.Id, domain.TokenTypeAccess)
		_ = r.Token.DeleteByUserId(ctx, user.Id, domain.TokenTypeRefresh)
		if err := r.Token.Create(ctx, accessTokenEntity); err != nil {
			return err
		}
		return r.Token.Create(ctx, refreshTokenEntity)
	}); err != nil {
		return nil, "", "", err
	}

	user.Password = ""

	return user, accessToken, refreshToken, nil
}

func (a *AuthUseCase) RefreshToken(ctx context.Context, refreshToken string) (string, string, error) {
	claims, err := a.jwtService.ValidateRefreshToken(refreshToken)
	if err != nil {
		return "", "", errors.New("неверный токен обновления")
	}

	token, err := a.tokenRepo.GetByToken(ctx, refreshToken)
	if err != nil || token.IsExpired() {
		return "", "", errors.New("неверный токен обновления")
	}

	user, err := a.userRepo.GetById(ctx, claims.UserId)
	if err != nil {
		return "", "", errors.New("пользователь не найден")
	}

	accessToken, accessExpires, err := a.jwtService.GenerateAccessToken(user)
	if err != nil {
		return "", "", err
	}

	newRefreshToken, refreshExpires, err := a.jwtService.GenerateRefreshToken(user)
	if err != nil {
		return "", "", err
	}

	accessTokenEntity := domain.NewToken(user.Id, accessToken, domain.TokenTypeAccess, accessExpires)
	refreshTokenEntity := domain.NewToken(user.Id, newRefreshToken, domain.TokenTypeRefresh, refreshExpires)

	if err := a.authTx.WithinTx(ctx, func(ctx context.Context, r domain.AuthRepos) error {
		_ = r.Token.DeleteByUserId(ctx, user.Id, domain.TokenTypeAccess)
		_ = r.Token.DeleteByToken(ctx, refreshToken)
		if err := r.Token.Create(ctx, accessTokenEntity); err != nil {
			return err
		}
		return r.Token.Create(ctx, refreshTokenEntity)
	}); err != nil {
		return "", "", err
	}

	return accessToken, newRefreshToken, nil
}

func (a *AuthUseCase) ValidateToken(ctx context.Context, token string) (*domain.User, error) {
	claims, err := a.jwtService.ValidateAccessToken(token)
	if err != nil {
		return nil, errors.New("неверный токен")
	}

	tokenEntity, err := a.tokenRepo.GetByToken(ctx, token)
	if err != nil || tokenEntity.IsExpired() {
		return nil, errors.New("неверный токен")
	}

	user, err := a.userRepo.GetById(ctx, claims.UserId)
	if err != nil {
		return nil, errors.New("пользователь не найден")
	}

	_ = a.userRepo.UpdateLastVisitedAt(ctx, user.Id)

	user.Password = ""

	return user, nil
}

func (a *AuthUseCase) Logout(ctx context.Context, userId int) error {
	return a.authTx.WithinTx(ctx, func(ctx context.Context, r domain.AuthRepos) error {
		if err := r.Token.DeleteByUserId(ctx, userId, domain.TokenTypeAccess); err != nil {
			return err
		}
		return r.Token.DeleteByUserId(ctx, userId, domain.TokenTypeRefresh)
	})
}

func (a *AuthUseCase) ChangePassword(ctx context.Context, userId int, oldPassword, newPassword string) error {
	if oldPassword == "" {
		return errors.New("текущий пароль не может быть пустым")
	}
	if err := pkg.ValidatePassword(newPassword); err != nil {
		return err
	}

	user, err := a.userRepo.GetById(ctx, userId)
	if err != nil {
		return err
	}

	if !a.jwtService.CheckPassword(user.Password, oldPassword) {
		return errors.New("неверный текущий пароль")
	}

	hashed, err := a.jwtService.HashPassword(newPassword)
	if err != nil {
		return err
	}

	user.Password = hashed
	return a.authTx.WithinTx(ctx, func(ctx context.Context, r domain.AuthRepos) error {
		if err := r.User.Update(ctx, user); err != nil {
			return err
		}
		_ = r.Token.DeleteByUserId(ctx, userId, domain.TokenTypeAccess)
		_ = r.Token.DeleteByUserId(ctx, userId, domain.TokenTypeRefresh)
		return nil
	})
}
