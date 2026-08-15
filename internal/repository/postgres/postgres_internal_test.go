package postgres

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsUniqueViolation(t *testing.T) {
	err := fmt.Errorf("insert user: %w", &pgconn.PgError{Code: "23505", ConstraintName: "users_login_key"})

	assert.True(t, isUniqueViolation(err, "users_login_key"))
	assert.True(t, isUniqueViolation(err, ""))
	assert.False(t, isUniqueViolation(err, "orders_pkey"))
	assert.False(t, isUniqueViolation(&pgconn.PgError{Code: "23503"}, ""))
	assert.False(t, isUniqueViolation(errors.New("boom"), ""))
}

func TestIsRetriable(t *testing.T) {
	assert.True(t, isRetriable(fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "08006"})))
	assert.True(t, isRetriable(&net.OpError{Op: "dial", Err: errors.New("connection refused")}))
	assert.False(t, isRetriable(&pgconn.PgError{Code: "23505"}))
	assert.False(t, isRetriable(context.Canceled))
	assert.False(t, isRetriable(context.DeadlineExceeded))
	assert.False(t, isRetriable(nil))
}

func TestWithRetrySucceedsAfterTransientFailures(t *testing.T) {
	originalDelays := retryDelays
	retryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryDelays = originalDelays })

	attempts := 0
	err := withRetry(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return &pgconn.PgError{Code: "08003"}
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestWithRetryDoesNotRepeatPermanentErrors(t *testing.T) {
	attempts := 0
	err := withRetry(context.Background(), func() error {
		attempts++
		return &pgconn.PgError{Code: "23505"}
	})

	assert.Error(t, err)
	assert.Equal(t, 1, attempts)
}

func TestWithRetryStopsOnCanceledContext(t *testing.T) {
	originalDelays := retryDelays
	retryDelays = []time.Duration{time.Hour}
	t.Cleanup(func() { retryDelays = originalDelays })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := withRetry(ctx, func() error { return &pgconn.PgError{Code: "08006"} })
	assert.ErrorIs(t, err, context.Canceled)
}

func TestNewPoolRejectsBrokenDSN(t *testing.T) {
	_, err := NewPool(context.Background(), "://not-a-dsn")
	assert.ErrorContains(t, err, "parse database uri")
}
