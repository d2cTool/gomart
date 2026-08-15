package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/d2cTool/gomart/internal/models"
	"github.com/d2cTool/gomart/internal/service"
)

// errStorage — искусственная ошибка хранилища для проверки проброса ошибок.
var errStorage = errors.New("storage is down")

// userRepoStub — подставная реализация service.UserRepository.
type userRepoStub struct {
	created   models.User
	createErr error
	found     models.User
	getErr    error
}

func (s *userRepoStub) Create(_ context.Context, login, passwordHash string) (models.User, error) {
	if s.createErr != nil {
		return models.User{}, s.createErr
	}
	s.created = models.User{ID: 1, Login: login, PasswordHash: passwordHash}
	return s.created, nil
}

func (s *userRepoStub) GetByLogin(_ context.Context, _ string) (models.User, error) {
	if s.getErr != nil {
		return models.User{}, s.getErr
	}
	return s.found, nil
}

// tokenIssuerStub — подставная реализация service.TokenIssuer.
type tokenIssuerStub struct {
	token string
	err   error
}

func (s tokenIssuerStub) Issue(int64) (string, error) { return s.token, s.err }

func TestAuthServiceRegister(t *testing.T) {
	users := &userRepoStub{}
	svc := service.NewAuthService(users, tokenIssuerStub{token: "token"})

	token, err := svc.Register(context.Background(), "alice", "s3cret")
	require.NoError(t, err)
	assert.Equal(t, "token", token)
	assert.Equal(t, "alice", users.created.Login)
	assert.NotEqual(t, "s3cret", users.created.PasswordHash)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(users.created.PasswordHash), []byte("s3cret")))
}

func TestAuthServiceRegisterEmptyCredentials(t *testing.T) {
	svc := service.NewAuthService(&userRepoStub{}, tokenIssuerStub{token: "token"})

	_, err := svc.Register(context.Background(), "   ", "s3cret")
	assert.ErrorIs(t, err, models.ErrInvalidCredentials)

	_, err = svc.Register(context.Background(), "alice", "")
	assert.ErrorIs(t, err, models.ErrInvalidCredentials)
}

func TestAuthServiceRegisterLoginTaken(t *testing.T) {
	svc := service.NewAuthService(&userRepoStub{createErr: models.ErrLoginTaken}, tokenIssuerStub{token: "token"})

	_, err := svc.Register(context.Background(), "alice", "s3cret")
	assert.ErrorIs(t, err, models.ErrLoginTaken)
}

func TestAuthServiceRegisterTokenError(t *testing.T) {
	svc := service.NewAuthService(&userRepoStub{}, tokenIssuerStub{err: errStorage})

	_, err := svc.Register(context.Background(), "alice", "s3cret")
	assert.ErrorIs(t, err, errStorage)
}

func TestAuthServiceLogin(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	require.NoError(t, err)

	users := &userRepoStub{found: models.User{ID: 7, Login: "alice", PasswordHash: string(hash)}}
	svc := service.NewAuthService(users, tokenIssuerStub{token: "token"})

	token, err := svc.Login(context.Background(), "alice", "s3cret")
	require.NoError(t, err)
	assert.Equal(t, "token", token)
}

func TestAuthServiceLoginWrongPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	require.NoError(t, err)

	users := &userRepoStub{found: models.User{ID: 7, PasswordHash: string(hash)}}
	svc := service.NewAuthService(users, tokenIssuerStub{token: "token"})

	_, err = svc.Login(context.Background(), "alice", "wrong")
	assert.ErrorIs(t, err, models.ErrInvalidCredentials)
}

func TestAuthServiceLoginUnknownUser(t *testing.T) {
	svc := service.NewAuthService(&userRepoStub{getErr: models.ErrUserNotFound}, tokenIssuerStub{token: "token"})

	_, err := svc.Login(context.Background(), "bob", "s3cret")
	assert.ErrorIs(t, err, models.ErrInvalidCredentials)
}

func TestAuthServiceLoginStorageError(t *testing.T) {
	svc := service.NewAuthService(&userRepoStub{getErr: errStorage}, tokenIssuerStub{token: "token"})

	_, err := svc.Login(context.Background(), "bob", "s3cret")
	assert.ErrorIs(t, err, errStorage)
}

func TestAuthServiceLoginEmptyCredentials(t *testing.T) {
	svc := service.NewAuthService(&userRepoStub{}, tokenIssuerStub{token: "token"})

	_, err := svc.Login(context.Background(), "", "s3cret")
	assert.ErrorIs(t, err, models.ErrInvalidCredentials)
}

// orderRepoStub — подставная реализация service.OrderRepository.
type orderRepoStub struct {
	createdNumber string
	createdUserID int64
	createErr     error
	orders        []models.Order
	listErr       error
}

func (s *orderRepoStub) Create(_ context.Context, userID int64, number string) error {
	s.createdUserID, s.createdNumber = userID, number
	return s.createErr
}

func (s *orderRepoStub) ListByUser(context.Context, int64) ([]models.Order, error) {
	return s.orders, s.listErr
}

func TestOrderServiceUpload(t *testing.T) {
	orders := &orderRepoStub{}
	svc := service.NewOrderService(orders)

	require.NoError(t, svc.Upload(context.Background(), 7, "12345678903"))
	assert.Equal(t, int64(7), orders.createdUserID)
	assert.Equal(t, "12345678903", orders.createdNumber)
}

func TestOrderServiceUploadInvalidNumber(t *testing.T) {
	orders := &orderRepoStub{}
	svc := service.NewOrderService(orders)

	err := svc.Upload(context.Background(), 7, "12345678901")
	assert.ErrorIs(t, err, models.ErrInvalidOrderNumber)
	assert.Empty(t, orders.createdNumber)
}

func TestOrderServiceUploadConflict(t *testing.T) {
	svc := service.NewOrderService(&orderRepoStub{createErr: models.ErrOrderOwnedByAnother})

	err := svc.Upload(context.Background(), 7, "12345678903")
	assert.ErrorIs(t, err, models.ErrOrderOwnedByAnother)
}

func TestOrderServiceList(t *testing.T) {
	want := []models.Order{{Number: "12345678903", Status: models.StatusNew, UploadedAt: time.Now()}}
	svc := service.NewOrderService(&orderRepoStub{orders: want})

	got, err := svc.List(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOrderServiceListError(t *testing.T) {
	svc := service.NewOrderService(&orderRepoStub{listErr: errStorage})

	_, err := svc.List(context.Background(), 7)
	assert.ErrorIs(t, err, errStorage)
}

// balanceRepoStub — подставная реализация service.BalanceRepository.
type balanceRepoStub struct {
	balance      models.Balance
	getErr       error
	withdrawErr  error
	withdrawnSum models.Points
	withdrawn    string
	withdrawals  []models.Withdrawal
	listErr      error
}

func (s *balanceRepoStub) Get(context.Context, int64) (models.Balance, error) {
	return s.balance, s.getErr
}

func (s *balanceRepoStub) Withdraw(_ context.Context, _ int64, order string, sum models.Points) error {
	s.withdrawn, s.withdrawnSum = order, sum
	return s.withdrawErr
}

func (s *balanceRepoStub) ListWithdrawals(context.Context, int64) ([]models.Withdrawal, error) {
	return s.withdrawals, s.listErr
}

func TestBalanceServiceGet(t *testing.T) {
	want := models.Balance{Current: models.NewPoints(500.5), Withdrawn: models.NewPoints(42)}
	svc := service.NewBalanceService(&balanceRepoStub{balance: want})

	got, err := svc.Get(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestBalanceServiceWithdraw(t *testing.T) {
	balances := &balanceRepoStub{}
	svc := service.NewBalanceService(balances)

	require.NoError(t, svc.Withdraw(context.Background(), 7, "2377225624", models.NewPoints(751)))
	assert.Equal(t, "2377225624", balances.withdrawn)
	assert.Equal(t, models.NewPoints(751), balances.withdrawnSum)
}

func TestBalanceServiceWithdrawInvalidInput(t *testing.T) {
	svc := service.NewBalanceService(&balanceRepoStub{})

	assert.ErrorIs(t, svc.Withdraw(context.Background(), 7, "123", models.NewPoints(10)), models.ErrInvalidOrderNumber)
	assert.ErrorIs(t, svc.Withdraw(context.Background(), 7, "2377225624", 0), models.ErrInvalidOrderNumber)
	assert.ErrorIs(t, svc.Withdraw(context.Background(), 7, "2377225624", models.NewPoints(-1)), models.ErrInvalidOrderNumber)
}

func TestBalanceServiceWithdrawInsufficientFunds(t *testing.T) {
	svc := service.NewBalanceService(&balanceRepoStub{withdrawErr: models.ErrInsufficientFunds})

	err := svc.Withdraw(context.Background(), 7, "2377225624", models.NewPoints(751))
	assert.ErrorIs(t, err, models.ErrInsufficientFunds)
}

func TestBalanceServiceWithdrawals(t *testing.T) {
	want := []models.Withdrawal{{Order: "2377225624", Sum: models.NewPoints(500), ProcessedAt: time.Now()}}
	svc := service.NewBalanceService(&balanceRepoStub{withdrawals: want})

	got, err := svc.Withdrawals(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestBalanceServiceWithdrawalsError(t *testing.T) {
	svc := service.NewBalanceService(&balanceRepoStub{listErr: errStorage})

	_, err := svc.Withdrawals(context.Background(), 7)
	assert.ErrorIs(t, err, errStorage)
}
