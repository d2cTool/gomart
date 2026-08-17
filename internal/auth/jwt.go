// Package auth отвечает за выпуск и проверку JWT-токенов, которыми
// подтверждается аутентификация пользователя системы лояльности.
package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken возвращается, если токен повреждён, просрочен или подписан
// другим ключом.
var ErrInvalidToken = errors.New("invalid authentication token")

// TokenManager выпускает и проверяет токены аутентификации.
type TokenManager struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenManager создаёт менеджер токенов с заданными секретом подписи и
// сроком жизни токена.
func NewTokenManager(secret string, ttl time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), ttl: ttl}
}

// Issue выпускает подписанный токен для пользователя с идентификатором userID.
func (m *TokenManager) Issue(userID int64) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return token, nil
}

// Parse проверяет подпись и срок действия токена и возвращает идентификатор
// пользователя, которому он выдан.
func (m *TokenManager) Parse(token string) (int64, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !parsed.Valid {
		return 0, ErrInvalidToken
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, ErrInvalidToken
	}
	return userID, nil
}
