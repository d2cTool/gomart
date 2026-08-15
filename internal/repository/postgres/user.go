package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/d2cTool/gomart/internal/models"
)

// UserRepository хранит пользователей системы лояльности в PostgreSQL.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository создаёт репозиторий пользователей поверх пула соединений.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create регистрирует нового пользователя вместе с его накопительным счётом.
// Если логин уже занят, возвращается models.ErrLoginTaken.
func (r *UserRepository) Create(ctx context.Context, login, passwordHash string) (models.User, error) {
	var user models.User
	err := withRetry(ctx, func() error {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		row := tx.QueryRow(ctx,
			`INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id, login, password_hash, created_at`,
			login, passwordHash)
		var u models.User
		if err := row.Scan(&u.ID, &u.Login, &u.PasswordHash, &u.CreatedAt); err != nil {
			if isUniqueViolation(err, "users_login_key") {
				return models.ErrLoginTaken
			}
			return fmt.Errorf("insert user: %w", err)
		}

		if _, err := tx.Exec(ctx, `INSERT INTO balances (user_id) VALUES ($1)`, u.ID); err != nil {
			return fmt.Errorf("insert balance: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit tx: %w", err)
		}
		user = u
		return nil
	})
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

// GetByLogin возвращает пользователя по логину.
// Если пользователь не найден, возвращается models.ErrUserNotFound.
func (r *UserRepository) GetByLogin(ctx context.Context, login string) (models.User, error) {
	var user models.User
	err := withRetry(ctx, func() error {
		row := r.pool.QueryRow(ctx,
			`SELECT id, login, password_hash, created_at FROM users WHERE login = $1`, login)
		if err := row.Scan(&user.ID, &user.Login, &user.PasswordHash, &user.CreatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return models.ErrUserNotFound
			}
			return fmt.Errorf("select user: %w", err)
		}
		return nil
	})
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}
