package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/d2cTool/gomart/internal/models"
)

// withdrawRequest — тело запроса на списание баллов.
type withdrawRequest struct {
	// Order — номер заказа, в счёт которого списываются баллы.
	Order string `json:"order"`
	// Sum — количество списываемых баллов.
	Sum models.Points `json:"sum"`
}

// GetBalance обрабатывает GET /api/user/balance: отдаёт текущий баланс
// и сумму списаний пользователя.
//
// Коды ответа: 200 — успех, 401 — пользователь не аутентифицирован,
// 500 — внутренняя ошибка.
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	balance, err := h.balances.Get(r.Context(), userID)
	if err != nil {
		h.internalError(w, "get balance", err)
		return
	}
	h.writeJSON(w, http.StatusOK, balance)
}

// Withdraw обрабатывает POST /api/user/balance/withdraw: списывает баллы
// в счёт оплаты нового заказа.
//
// Коды ответа: 200 — успех, 401 — пользователь не аутентифицирован,
// 402 — недостаточно средств, 422 — неверный номер заказа,
// 500 — внутренняя ошибка.
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	var req withdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	err := h.balances.Withdraw(r.Context(), userID, req.Order, req.Sum)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, models.ErrInsufficientFunds):
		http.Error(w, "insufficient funds", http.StatusPaymentRequired)
	case errors.Is(err, models.ErrInvalidOrderNumber), errors.Is(err, models.ErrWithdrawalExists):
		http.Error(w, "invalid order number", http.StatusUnprocessableEntity)
	default:
		h.internalError(w, "withdraw points", err)
	}
}

// ListWithdrawals обрабатывает GET /api/user/withdrawals: отдаёт историю
// списаний пользователя, отсортированную от новых к старым.
//
// Коды ответа: 200 — успех, 204 — списаний не было,
// 401 — пользователь не аутентифицирован, 500 — внутренняя ошибка.
func (h *Handler) ListWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	withdrawals, err := h.balances.Withdrawals(r.Context(), userID)
	if err != nil {
		h.internalError(w, "list withdrawals", err)
		return
	}
	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.writeJSON(w, http.StatusOK, withdrawals)
}
