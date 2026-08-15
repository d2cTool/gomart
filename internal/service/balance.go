package service

import (
	"context"

	"github.com/d2cTool/gomart/internal/luhn"
	"github.com/d2cTool/gomart/internal/models"
)

// BalanceRepository — хранилище накопительных счетов и списаний.
type BalanceRepository interface {
	// Get возвращает баланс пользователя.
	Get(ctx context.Context, userID int64) (models.Balance, error)
	// Withdraw списывает баллы, возвращая models.ErrInsufficientFunds,
	// если средств недостаточно.
	Withdraw(ctx context.Context, userID int64, order string, sum models.Points) error
	// ListWithdrawals возвращает списания пользователя от новых к старым.
	ListWithdrawals(ctx context.Context, userID int64) ([]models.Withdrawal, error)
}

// BalanceService реализует операции с накопительным счётом пользователя.
type BalanceService struct {
	balances BalanceRepository
}

// NewBalanceService создаёт сервис накопительного счёта.
func NewBalanceService(balances BalanceRepository) *BalanceService {
	return &BalanceService{balances: balances}
}

// Get возвращает текущий баланс и сумму списаний пользователя.
func (s *BalanceService) Get(ctx context.Context, userID int64) (models.Balance, error) {
	return s.balances.Get(ctx, userID)
}

// Withdraw списывает баллы в счёт оплаты заказа. Номер заказа проверяется по
// алгоритму Луна, сумма списания должна быть положительной, иначе
// возвращается models.ErrInvalidOrderNumber.
func (s *BalanceService) Withdraw(ctx context.Context, userID int64, order string, sum models.Points) error {
	if !luhn.IsValid(order) {
		return models.ErrInvalidOrderNumber
	}
	if sum <= 0 {
		return models.ErrInvalidOrderNumber
	}
	return s.balances.Withdraw(ctx, userID, order, sum)
}

// Withdrawals возвращает историю списаний пользователя от новых к старым.
func (s *BalanceService) Withdrawals(ctx context.Context, userID int64) ([]models.Withdrawal, error) {
	return s.balances.ListWithdrawals(ctx, userID)
}
