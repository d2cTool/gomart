package models

import "errors"

// Доменные ошибки системы лояльности. Обработчики HTTP-слоя опираются на них
// при выборе кода ответа.
var (
	// ErrLoginTaken возвращается при попытке зарегистрировать занятый логин.
	ErrLoginTaken = errors.New("login is already taken")
	// ErrInvalidCredentials возвращается при неверной паре логин/пароль.
	ErrInvalidCredentials = errors.New("invalid login/password pair")
	// ErrUserNotFound возвращается, если пользователь не найден.
	ErrUserNotFound = errors.New("user not found")
	// ErrOrderAlreadyUploaded возвращается, если заказ уже загружен тем же пользователем.
	ErrOrderAlreadyUploaded = errors.New("order already uploaded by this user")
	// ErrOrderOwnedByAnother возвращается, если заказ загружен другим пользователем.
	ErrOrderOwnedByAnother = errors.New("order already uploaded by another user")
	// ErrInvalidOrderNumber возвращается при номере заказа, не проходящем проверку Луна.
	ErrInvalidOrderNumber = errors.New("invalid order number")
	// ErrInsufficientFunds возвращается, если на счету недостаточно баллов.
	ErrInsufficientFunds = errors.New("insufficient funds on the loyalty account")
	// ErrWithdrawalExists возвращается, если по этому номеру заказа списание уже зарегистрировано.
	ErrWithdrawalExists = errors.New("withdrawal for this order already registered")
	// ErrOrderNotRegistered возвращается, если заказ неизвестен системе расчёта начислений.
	ErrOrderNotRegistered = errors.New("order is not registered in the accrual system")
)

// TooManyRequestsError сообщает о превышении лимита запросов к системе расчёта
// начислений и содержит рекомендованную паузу перед следующей попыткой.
type TooManyRequestsError struct {
	// RetryAfterSeconds — пауза в секундах из заголовка Retry-After.
	RetryAfterSeconds int
	// Message — текст ответа системы расчёта начислений.
	Message string
}

// Error реализует интерфейс error.
func (e *TooManyRequestsError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "too many requests to the accrual system"
}
