//go:build integration

// Интеграционные тесты хранилища. Запускаются с тегом integration и требуют
// доступного экземпляра PostgreSQL, адрес которого передаётся в переменной
// окружения TEST_DATABASE_URI:
//
//	go test -tags=integration ./...
package postgres_test

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/d2cTool/gomart/internal/models"
	"github.com/d2cTool/gomart/internal/repository/postgres"
)

// testSchema — отдельная схема БД, чтобы тесты пакета не мешали тестам
// других пакетов, работающим с той же базой.
const testSchema = "test_repository"

// schemaDSN добавляет к строке подключения переключение на указанную схему.
func schemaDSN(t *testing.T, base, schema string) string {
	t.Helper()
	u, err := url.Parse(base)
	require.NoError(t, err)
	q := u.Query()
	q.Set("options", "-c search_path="+schema)
	u.RawQuery = q.Encode()
	return u.String()
}

// newTestPool поднимает пул соединений с тестовой схемой и очищает её данные.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	base := os.Getenv("TEST_DATABASE_URI")
	if base == "" {
		t.Skip("TEST_DATABASE_URI is not set, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := postgres.NewPool(ctx, base)
	require.NoError(t, err)
	_, err = admin.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+testSchema)
	admin.Close()
	require.NoError(t, err)

	pool, err := postgres.NewPool(ctx, schemaDSN(t, base, testSchema))
	require.NoError(t, err)
	require.NoError(t, postgres.Migrate(ctx, pool))

	_, err = pool.Exec(ctx, `TRUNCATE withdrawals, orders, balances, users RESTART IDENTITY CASCADE`)
	require.NoError(t, err)

	t.Cleanup(pool.Close)
	return pool
}

// createUser регистрирует пользователя и возвращает его идентификатор.
func createUser(t *testing.T, pool *pgxpool.Pool, login string) int64 {
	t.Helper()
	user, err := postgres.NewUserRepository(pool).Create(context.Background(), login, "hash")
	require.NoError(t, err)
	return user.ID
}

func TestUserRepositoryCreateAndGet(t *testing.T) {
	pool := newTestPool(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	created, err := repo.Create(ctx, "alice", "hash")
	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, "alice", created.Login)

	found, err := repo.GetByLogin(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, "hash", found.PasswordHash)

	_, err = repo.Create(ctx, "alice", "another-hash")
	assert.ErrorIs(t, err, models.ErrLoginTaken)

	_, err = repo.GetByLogin(ctx, "bob")
	assert.ErrorIs(t, err, models.ErrUserNotFound)
}

func TestUserRepositoryCreatesBalance(t *testing.T) {
	pool := newTestPool(t)
	userID := createUser(t, pool, "alice")

	balance, err := postgres.NewBalanceRepository(pool).Get(context.Background(), userID)
	require.NoError(t, err)
	assert.Equal(t, models.Points(0), balance.Current)
	assert.Equal(t, models.Points(0), balance.Withdrawn)
}

func TestOrderRepositoryCreateConflicts(t *testing.T) {
	pool := newTestPool(t)
	repo := postgres.NewOrderRepository(pool)
	ctx := context.Background()

	alice := createUser(t, pool, "alice")
	bob := createUser(t, pool, "bob")

	require.NoError(t, repo.Create(ctx, alice, "12345678903"))
	assert.ErrorIs(t, repo.Create(ctx, alice, "12345678903"), models.ErrOrderAlreadyUploaded)
	assert.ErrorIs(t, repo.Create(ctx, bob, "12345678903"), models.ErrOrderOwnedByAnother)
}

func TestOrderRepositoryListByUserSorting(t *testing.T) {
	pool := newTestPool(t)
	repo := postgres.NewOrderRepository(pool)
	ctx := context.Background()
	userID := createUser(t, pool, "alice")

	require.NoError(t, repo.Create(ctx, userID, "12345678903"))
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, repo.Create(ctx, userID, "9278923470"))

	orders, err := repo.ListByUser(ctx, userID)
	require.NoError(t, err)
	require.Len(t, orders, 2)
	assert.Equal(t, "9278923470", orders[0].Number)
	assert.Equal(t, models.StatusNew, orders[0].Status)
	assert.Nil(t, orders[0].Accrual)
	assert.Equal(t, "12345678903", orders[1].Number)

	empty, err := repo.ListByUser(ctx, createUser(t, pool, "bob"))
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestOrderRepositoryTakePendingAndApplyAccrual(t *testing.T) {
	pool := newTestPool(t)
	orders := postgres.NewOrderRepository(pool)
	balances := postgres.NewBalanceRepository(pool)
	ctx := context.Background()
	userID := createUser(t, pool, "alice")

	require.NoError(t, orders.Create(ctx, userID, "12345678903"))
	require.NoError(t, orders.Create(ctx, userID, "9278923470"))

	pending, err := orders.TakePending(ctx, 10)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"12345678903", "9278923470"}, pending)

	require.NoError(t, orders.ApplyAccrual(ctx, "12345678903", models.StatusProcessed, models.NewPoints(500.5)))
	require.NoError(t, orders.ApplyAccrual(ctx, "9278923470", models.StatusInvalid, 0))

	balance, err := balances.Get(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, models.NewPoints(500.5), balance.Current)

	pending, err = orders.TakePending(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, pending, "заказы в окончательных статусах не должны опрашиваться повторно")

	list, err := orders.ListByUser(ctx, userID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	for _, order := range list {
		if order.Number == "12345678903" {
			require.NotNil(t, order.Accrual)
			assert.Equal(t, models.NewPoints(500.5), *order.Accrual)
		}
	}
}

func TestOrderRepositoryApplyAccrualIsIdempotent(t *testing.T) {
	pool := newTestPool(t)
	orders := postgres.NewOrderRepository(pool)
	balances := postgres.NewBalanceRepository(pool)
	ctx := context.Background()
	userID := createUser(t, pool, "alice")

	require.NoError(t, orders.Create(ctx, userID, "12345678903"))
	require.NoError(t, orders.ApplyAccrual(ctx, "12345678903", models.StatusProcessed, models.NewPoints(100)))
	require.NoError(t, orders.ApplyAccrual(ctx, "12345678903", models.StatusProcessed, models.NewPoints(100)))

	balance, err := balances.Get(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, models.NewPoints(100), balance.Current, "повторный расчёт не должен начислять баллы дважды")
}

func TestBalanceRepositoryWithdraw(t *testing.T) {
	pool := newTestPool(t)
	orders := postgres.NewOrderRepository(pool)
	balances := postgres.NewBalanceRepository(pool)
	ctx := context.Background()
	userID := createUser(t, pool, "alice")

	require.NoError(t, orders.Create(ctx, userID, "12345678903"))
	require.NoError(t, orders.ApplyAccrual(ctx, "12345678903", models.StatusProcessed, models.NewPoints(1000)))

	assert.ErrorIs(t,
		balances.Withdraw(ctx, userID, "2377225624", models.NewPoints(1500)),
		models.ErrInsufficientFunds)

	require.NoError(t, balances.Withdraw(ctx, userID, "2377225624", models.NewPoints(751)))
	assert.ErrorIs(t,
		balances.Withdraw(ctx, userID, "2377225624", models.NewPoints(1)),
		models.ErrWithdrawalExists)

	balance, err := balances.Get(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, models.NewPoints(249), balance.Current)
	assert.Equal(t, models.NewPoints(751), balance.Withdrawn)

	withdrawals, err := balances.ListWithdrawals(ctx, userID)
	require.NoError(t, err)
	require.Len(t, withdrawals, 1)
	assert.Equal(t, "2377225624", withdrawals[0].Order)
	assert.Equal(t, models.NewPoints(751), withdrawals[0].Sum)
	assert.WithinDuration(t, time.Now(), withdrawals[0].ProcessedAt, time.Minute)
}

func TestBalanceRepositoryUnknownUser(t *testing.T) {
	pool := newTestPool(t)
	balances := postgres.NewBalanceRepository(pool)
	ctx := context.Background()

	_, err := balances.Get(ctx, 999)
	assert.ErrorIs(t, err, models.ErrUserNotFound)

	assert.ErrorIs(t, balances.Withdraw(ctx, 999, "2377225624", models.NewPoints(1)), models.ErrUserNotFound)
}
