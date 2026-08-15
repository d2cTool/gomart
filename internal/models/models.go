package models

import "time"

// OrderStatus — статус обработки заказа в системе лояльности.
type OrderStatus string

// Возможные статусы обработки заказа.
const (
	// StatusNew — заказ загружен в систему, но не попал в обработку.
	StatusNew OrderStatus = "NEW"
	// StatusProcessing — вознаграждение за заказ рассчитывается.
	StatusProcessing OrderStatus = "PROCESSING"
	// StatusInvalid — система расчёта вознаграждений отказала в расчёте.
	StatusInvalid OrderStatus = "INVALID"
	// StatusProcessed — расчёт по заказу завершён, начисление получено.
	StatusProcessed OrderStatus = "PROCESSED"
)

// IsFinal сообщает, является ли статус окончательным, то есть не требующим
// дальнейших обращений к системе расчёта начислений.
func (s OrderStatus) IsFinal() bool {
	return s == StatusInvalid || s == StatusProcessed
}

// User — зарегистрированный пользователь системы лояльности.
type User struct {
	// ID — внутренний идентификатор пользователя.
	ID int64
	// Login — уникальный логин пользователя.
	Login string
	// PasswordHash — bcrypt-хеш пароля пользователя.
	PasswordHash string
	// CreatedAt — момент регистрации пользователя.
	CreatedAt time.Time
}

// Order — заказ, загруженный пользователем в систему лояльности.
type Order struct {
	// Number — номер заказа.
	Number string `json:"number"`
	// Status — текущий статус обработки заказа.
	Status OrderStatus `json:"status"`
	// Accrual — начисленные за заказ баллы; отсутствует, если начислений нет.
	Accrual *Points `json:"accrual,omitempty"`
	// UploadedAt — момент загрузки заказа пользователем.
	UploadedAt time.Time `json:"uploaded_at"`
	// UserID — идентификатор владельца заказа, во внешнее API не отдаётся.
	UserID int64 `json:"-"`
}

// Balance — состояние накопительного счёта пользователя.
type Balance struct {
	// Current — текущее количество доступных баллов.
	Current Points `json:"current"`
	// Withdrawn — сумма баллов, списанных за всё время.
	Withdrawn Points `json:"withdrawn"`
}

// Withdrawal — факт списания баллов в счёт оплаты заказа.
type Withdrawal struct {
	// Order — номер заказа, в счёт которого произведено списание.
	Order string `json:"order"`
	// Sum — количество списанных баллов.
	Sum Points `json:"sum"`
	// ProcessedAt — момент списания.
	ProcessedAt time.Time `json:"processed_at"`
}

// AccrualStatus — статус расчёта во внешней системе начисления баллов.
type AccrualStatus string

// Возможные статусы расчёта во внешней системе начисления баллов.
const (
	// AccrualRegistered — заказ зарегистрирован, но вознаграждение не рассчитано.
	AccrualRegistered AccrualStatus = "REGISTERED"
	// AccrualInvalid — заказ не принят к расчёту.
	AccrualInvalid AccrualStatus = "INVALID"
	// AccrualProcessing — расчёт начисления выполняется.
	AccrualProcessing AccrualStatus = "PROCESSING"
	// AccrualProcessed — расчёт начисления окончен.
	AccrualProcessed AccrualStatus = "PROCESSED"
)

// AccrualInfo — ответ внешней системы расчёта начислений по одному заказу.
type AccrualInfo struct {
	// Order — номер заказа.
	Order string `json:"order"`
	// Status — статус расчёта начисления.
	Status AccrualStatus `json:"status"`
	// Accrual — рассчитанные баллы; отсутствуют, если начисления нет.
	Accrual *Points `json:"accrual,omitempty"`
}

// OrderStatus преобразует статус внешней системы в статус заказа
// системы лояльности.
func (s AccrualStatus) OrderStatus() OrderStatus {
	switch s {
	case AccrualProcessed:
		return StatusProcessed
	case AccrualInvalid:
		return StatusInvalid
	case AccrualProcessing, AccrualRegistered:
		return StatusProcessing
	default:
		return StatusProcessing
	}
}
