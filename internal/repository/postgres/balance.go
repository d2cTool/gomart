package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/d2cTool/gomart/internal/models"
)

// BalanceRepository хранит накопительные счета и списания в PostgreSQL.
type BalanceRepository struct {
	pool *pgxpool.Pool
}

// NewBalanceRepository создаёт репозиторий счетов поверх пула соединений.
func NewBalanceRepository(pool *pgxpool.Pool) *BalanceRepository {
	return &BalanceRepository{pool: pool}
}

// Get возвращает текущий баланс пользователя и сумму списаний за всё время.
func (r *BalanceRepository) Get(ctx context.Context, userID int64) (models.Balance, error) {
	var balance models.Balance
	err := withRetry(ctx, func() error {
		err := r.pool.QueryRow(ctx,
			`SELECT current, withdrawn FROM balances WHERE user_id = $1`, userID).
			Scan(&balance.Current, &balance.Withdrawn)
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("select balance: %w", err)
		}
		return nil
	})
	if err != nil {
		return models.Balance{}, err
	}
	return balance, nil
}

// Withdraw списывает баллы со счёта пользователя в счёт оплаты заказа.
// Возвращает models.ErrInsufficientFunds, если баллов недостаточно, и
// models.ErrWithdrawalExists, если списание по этому заказу уже проводилось.
func (r *BalanceRepository) Withdraw(ctx context.Context, userID int64, order string, sum models.Points) error {
	return withRetry(ctx, func() error {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		var current models.Points
		err = tx.QueryRow(ctx,
			`SELECT current FROM balances WHERE user_id = $1 FOR UPDATE`, userID).Scan(&current)
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("lock balance: %w", err)
		}
		if current < sum {
			return models.ErrInsufficientFunds
		}

		if _, err := tx.Exec(ctx,
			`UPDATE balances SET current = current - $2, withdrawn = withdrawn + $2 WHERE user_id = $1`,
			userID, sum); err != nil {
			return fmt.Errorf("debit balance: %w", err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO withdrawals (order_number, user_id, sum) VALUES ($1, $2, $3)`,
			order, userID, sum); err != nil {
			if isUniqueViolation(err, "withdrawals_order_number_key") {
				return models.ErrWithdrawalExists
			}
			return fmt.Errorf("insert withdrawal: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit tx: %w", err)
		}
		return nil
	})
}

// ListWithdrawals возвращает списания пользователя, отсортированные от самых
// новых к самым старым.
func (r *BalanceRepository) ListWithdrawals(ctx context.Context, userID int64) ([]models.Withdrawal, error) {
	var withdrawals []models.Withdrawal
	err := withRetry(ctx, func() error {
		rows, err := r.pool.Query(ctx,
			`SELECT order_number, sum, processed_at
			 FROM withdrawals WHERE user_id = $1 ORDER BY processed_at DESC`, userID)
		if err != nil {
			return fmt.Errorf("select withdrawals: %w", err)
		}
		defer rows.Close()

		result := make([]models.Withdrawal, 0)
		for rows.Next() {
			var w models.Withdrawal
			if err := rows.Scan(&w.Order, &w.Sum, &w.ProcessedAt); err != nil {
				return fmt.Errorf("scan withdrawal: %w", err)
			}
			result = append(result, w)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate withdrawals: %w", err)
		}
		withdrawals = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return withdrawals, nil
}
