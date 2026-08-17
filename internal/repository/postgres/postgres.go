// Package postgres содержит реализацию хранилища системы лояльности
// поверх PostgreSQL: пул соединений, миграции схемы и репозитории
// пользователей, заказов и накопительных счетов.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// uniqueViolationCode — код ошибки PostgreSQL при нарушении уникальности.
const uniqueViolationCode = "23505"

// migrationLockID — идентификатор рекомендательной блокировки, под которой
// применяются миграции. Блокировка не даёт нескольким экземплярам сервиса
// накатывать схему одновременно.
const migrationLockID int64 = 8_004_211_991

// retryDelays задаёт паузы между повторными попытками при обрыве соединения.
var retryDelays = []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}

// NewPool создаёт пул соединений с PostgreSQL и проверяет его доступность.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database uri: %w", err)
	}
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate применяет к базе данных все неприменённые миграции схемы.
// Миграции выполняются под рекомендательной блокировкой, поэтому одновременный
// запуск нескольких экземпляров сервиса безопасен.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for migrations: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrationLockID)
	}()

	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// isUniqueViolation сообщает, вызвана ли ошибка нарушением уникального индекса
// с указанным именем ограничения.
func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != uniqueViolationCode {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}

// isRetriable сообщает, имеет ли смысл повторить операцию: к таким ошибкам
// относятся сбои соединения с базой данных (класс 08 по SQLSTATE) и сетевые
// ошибки транспорта.
func isRetriable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return strings.HasPrefix(pgErr.Code, "08")
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// withRetry выполняет операцию, повторяя её при ошибках соединения с БД.
func withRetry(ctx context.Context, op func() error) error {
	err := op()
	for _, delay := range retryDelays {
		if !isRetriable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		err = op()
	}
	return err
}
