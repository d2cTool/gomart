package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/d2cTool/gomart/internal/auth"
)

func TestIssueAndParse(t *testing.T) {
	manager := auth.NewTokenManager("secret", time.Hour)

	token, err := manager.Issue(42)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	userID, err := manager.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, int64(42), userID)
}

func TestParseRejectsForeignSignature(t *testing.T) {
	token, err := auth.NewTokenManager("secret", time.Hour).Issue(1)
	require.NoError(t, err)

	_, err = auth.NewTokenManager("another-secret", time.Hour).Parse(token)
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestParseRejectsExpiredToken(t *testing.T) {
	manager := auth.NewTokenManager("secret", -time.Minute)

	token, err := manager.Issue(1)
	require.NoError(t, err)

	_, err = manager.Parse(token)
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestParseRejectsGarbage(t *testing.T) {
	manager := auth.NewTokenManager("secret", time.Hour)

	_, err := manager.Parse("not-a-token")
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestParseRejectsNonNumericSubject(t *testing.T) {
	claims := jwt.RegisteredClaims{
		Subject:   "admin",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
	require.NoError(t, err)

	_, err = auth.NewTokenManager("secret", time.Hour).Parse(token)
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestParseRejectsUnsignedToken(t *testing.T) {
	claims := jwt.RegisteredClaims{
		Subject:   "1",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = auth.NewTokenManager("secret", time.Hour).Parse(token)
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}
