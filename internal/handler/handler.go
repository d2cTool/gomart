// Package handler реализует HTTP API накопительной системы лояльности
// «Гофермарт» и маршрутизацию запросов к бизнес-логике сервиса.
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/d2cTool/gomart/internal/middleware"
	"github.com/d2cTool/gomart/internal/models"
)

// AuthService — бизнес-логика регистрации и аутентификации пользователей.
type AuthService interface {
	// Register регистрирует пользователя и возвращает токен аутентификации.
	Register(ctx context.Context, login, password string) (string, error)
	// Login проверяет пару логин/пароль и возвращает токен аутентификации.
	Login(ctx context.Context, login, password string) (string, error)
}

// OrderService — бизнес-логика приёма и выдачи номеров заказов.
type OrderService interface {
	// Upload принимает номер заказа от пользователя.
	Upload(ctx context.Context, userID int64, number string) error
	// List возвращает заказы пользователя от новых к старым.
	List(ctx context.Context, userID int64) ([]models.Order, error)
}

// BalanceService — бизнес-логика накопительного счёта пользователя.
type BalanceService interface {
	// Get возвращает баланс пользователя.
	Get(ctx context.Context, userID int64) (models.Balance, error)
	// Withdraw списывает баллы в счёт оплаты заказа.
	Withdraw(ctx context.Context, userID int64, order string, sum models.Points) error
	// Withdrawals возвращает историю списаний пользователя.
	Withdrawals(ctx context.Context, userID int64) ([]models.Withdrawal, error)
}

// Handler объединяет обработчики HTTP API системы лояльности.
type Handler struct {
	auth     AuthService
	orders   OrderService
	balances BalanceService
	parser   middleware.TokenParser
	log      *zap.Logger
}

// New создаёт набор обработчиков HTTP API.
func New(auth AuthService, orders OrderService, balances BalanceService, parser middleware.TokenParser, log *zap.Logger) *Handler {
	return &Handler{auth: auth, orders: orders, balances: balances, parser: parser, log: log}
}

// writeJSON отдаёт значение в формате JSON с указанным кодом ответа.
func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.log.Error("write json response", zap.Error(err))
	}
}

// internalError логирует ошибку и отдаёт клиенту 500.
func (h *Handler) internalError(w http.ResponseWriter, msg string, err error) {
	h.log.Error(msg, zap.Error(err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// userID возвращает идентификатор аутентифицированного пользователя.
// Если пользователь не аутентифицирован, клиенту отдаётся 401.
func (h *Handler) userID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return 0, false
	}
	return userID, true
}
