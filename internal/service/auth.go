// Package service содержит бизнес-логику накопительной системы лояльности:
// регистрацию и аутентификацию пользователей, приём заказов и операции
// с накопительным счётом.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/d2cTool/gomart/internal/models"
)

// UserRepository — хранилище пользователей, необходимое сервису аутентификации.
type UserRepository interface {
	// Create сохраняет нового пользователя, возвращая models.ErrLoginTaken,
	// если логин уже занят.
	Create(ctx context.Context, login, passwordHash string) (models.User, error)
	// GetByLogin возвращает пользователя по логину или models.ErrUserNotFound.
	GetByLogin(ctx context.Context, login string) (models.User, error)
}

// TokenIssuer выпускает токены аутентификации.
type TokenIssuer interface {
	// Issue выпускает токен для пользователя с идентификатором userID.
	Issue(userID int64) (string, error)
}

// AuthService реализует регистрацию и аутентификацию пользователей.
type AuthService struct {
	users  UserRepository
	tokens TokenIssuer
}

// NewAuthService создаёт сервис аутентификации.
func NewAuthService(users UserRepository, tokens TokenIssuer) *AuthService {
	return &AuthService{users: users, tokens: tokens}
}

// Register регистрирует пользователя и сразу возвращает токен аутентификации.
// Возвращает models.ErrInvalidCredentials при пустом логине или пароле и
// models.ErrLoginTaken, если логин занят.
func (s *AuthService) Register(ctx context.Context, login, password string) (string, error) {
	if strings.TrimSpace(login) == "" || password == "" {
		return "", models.ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	user, err := s.users.Create(ctx, login, string(hash))
	if err != nil {
		return "", err
	}

	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return "", fmt.Errorf("issue token: %w", err)
	}
	return token, nil
}

// Login проверяет пару логин/пароль и возвращает токен аутентификации.
// При неверных данных возвращается models.ErrInvalidCredentials.
func (s *AuthService) Login(ctx context.Context, login, password string) (string, error) {
	if strings.TrimSpace(login) == "" || password == "" {
		return "", models.ErrInvalidCredentials
	}

	user, err := s.users.GetByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, models.ErrUserNotFound) {
			return "", models.ErrInvalidCredentials
		}
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", models.ErrInvalidCredentials
	}

	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return "", fmt.Errorf("issue token: %w", err)
	}
	return token, nil
}
