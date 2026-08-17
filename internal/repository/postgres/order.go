package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/d2cTool/gomart/internal/models"
)

// OrderRepository хранит заказы пользователей в PostgreSQL.
type OrderRepository struct {
	pool *pgxpool.Pool
}

// NewOrderRepository создаёт репозиторий заказов поверх пула соединений.
func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

// Create принимает номер заказа от пользователя. Возвращает
// models.ErrOrderAlreadyUploaded, если заказ уже был загружен этим же
// пользователем, и models.ErrOrderOwnedByAnother, если заказ принадлежит
// другому пользователю.
func (r *OrderRepository) Create(ctx context.Context, userID int64, number string) error {
	const query = `
		WITH inserted AS (
			INSERT INTO orders (number, user_id)
			VALUES ($1, $2)
			ON CONFLICT (number) DO NOTHING
			RETURNING user_id
		)
		SELECT user_id, true FROM inserted
		UNION ALL
		SELECT user_id, false
		FROM orders
		WHERE number = $1 AND NOT EXISTS (SELECT 1 FROM inserted)`

	return withRetry(ctx, func() error {
		var (
			ownerID  int64
			inserted bool
		)
		if err := r.pool.QueryRow(ctx, query, number, userID).Scan(&ownerID, &inserted); err != nil {
			return fmt.Errorf("upsert order: %w", err)
		}
		if inserted {
			return nil
		}
		if ownerID == userID {
			return models.ErrOrderAlreadyUploaded
		}
		return models.ErrOrderOwnedByAnother
	})
}

// ListByUser возвращает заказы пользователя, отсортированные от самых новых
// к самым старым.
func (r *OrderRepository) ListByUser(ctx context.Context, userID int64) ([]models.Order, error) {
	var orders []models.Order
	err := withRetry(ctx, func() error {
		rows, err := r.pool.Query(ctx,
			`SELECT number, status, accrual, uploaded_at
			 FROM orders WHERE user_id = $1 ORDER BY uploaded_at DESC`, userID)
		if err != nil {
			return fmt.Errorf("select orders: %w", err)
		}
		defer rows.Close()

		result := make([]models.Order, 0)
		for rows.Next() {
			var (
				order   models.Order
				accrual models.Points
			)
			if err := rows.Scan(&order.Number, &order.Status, &accrual, &order.UploadedAt); err != nil {
				return fmt.Errorf("scan order: %w", err)
			}
			order.UserID = userID
			if accrual > 0 {
				value := accrual
				order.Accrual = &value
			}
			result = append(result, order)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate orders: %w", err)
		}
		orders = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return orders, nil
}

// TakePending забирает до limit заказов в неокончательных статусах для опроса
// системы расчёта начислений. Строки блокируются с пропуском уже занятых
// (FOR UPDATE SKIP LOCKED), а отметка времени обновления сдвигается, чтобы
// один и тот же заказ не выбирался повторно подряд.
func (r *OrderRepository) TakePending(ctx context.Context, limit int) ([]string, error) {
	var numbers []string
	err := withRetry(ctx, func() error {
		rows, err := r.pool.Query(ctx,
			`UPDATE orders SET updated_at = now()
			 WHERE number IN (
			     SELECT number FROM orders
			     WHERE status IN ('NEW', 'PROCESSING')
			     ORDER BY updated_at
			     LIMIT $1
			     FOR UPDATE SKIP LOCKED
			 )
			 RETURNING number`, limit)
		if err != nil {
			return fmt.Errorf("take pending orders: %w", err)
		}
		defer rows.Close()

		result := make([]string, 0, limit)
		for rows.Next() {
			var number string
			if err := rows.Scan(&number); err != nil {
				return fmt.Errorf("scan pending order: %w", err)
			}
			result = append(result, number)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate pending orders: %w", err)
		}
		numbers = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return numbers, nil
}

// ApplyAccrual переводит заказ в новый статус и, если расчёт завершён
// начислением, атомарно пополняет накопительный счёт владельца заказа.
// Заказы, уже находящиеся в окончательном статусе, повторно не изменяются.
func (r *OrderRepository) ApplyAccrual(ctx context.Context, number string, status models.OrderStatus, accrual models.Points) error {
	return withRetry(ctx, func() error {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		var userID int64
		err = tx.QueryRow(ctx,
			`UPDATE orders SET status = $2, accrual = $3, updated_at = now()
			 WHERE number = $1 AND status NOT IN ('PROCESSED', 'INVALID')
			 RETURNING user_id`, number, status, accrual).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("update order status: %w", err)
		}

		if status == models.StatusProcessed && accrual > 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE balances SET current = current + $2 WHERE user_id = $1`,
				userID, accrual); err != nil {
				return fmt.Errorf("credit balance: %w", err)
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit tx: %w", err)
		}
		return nil
	})
}
