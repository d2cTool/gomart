package service

import (
	"context"

	"github.com/d2cTool/gomart/internal/luhn"
	"github.com/d2cTool/gomart/internal/models"
)

// OrderRepository — хранилище заказов, необходимое сервису заказов.
type OrderRepository interface {
	// Create сохраняет заказ за пользователем, возвращая
	// models.ErrOrderAlreadyUploaded или models.ErrOrderOwnedByAnother,
	// если номер заказа уже зарегистрирован.
	Create(ctx context.Context, userID int64, number string) error
	// ListByUser возвращает заказы пользователя от новых к старым.
	ListByUser(ctx context.Context, userID int64) ([]models.Order, error)
}

// OrderService реализует приём и выдачу номеров заказов пользователя.
type OrderService struct {
	orders OrderRepository
}

// NewOrderService создаёт сервис заказов.
func NewOrderService(orders OrderRepository) *OrderService {
	return &OrderService{orders: orders}
}

// Upload принимает номер заказа от пользователя. Номер проверяется по
// алгоритму Луна: при непрохождении проверки возвращается
// models.ErrInvalidOrderNumber.
func (s *OrderService) Upload(ctx context.Context, userID int64, number string) error {
	if !luhn.IsValid(number) {
		return models.ErrInvalidOrderNumber
	}
	return s.orders.Create(ctx, userID, number)
}

// List возвращает загруженные пользователем заказы от новых к старым.
func (s *OrderService) List(ctx context.Context, userID int64) ([]models.Order, error) {
	return s.orders.ListByUser(ctx, userID)
}
