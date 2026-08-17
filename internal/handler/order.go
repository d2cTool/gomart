package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/d2cTool/gomart/internal/models"
)

// UploadOrder обрабатывает POST /api/user/orders: принимает номер заказа
// в формате text/plain.
//
// Коды ответа: 200 — заказ уже загружен этим пользователем,
// 202 — заказ принят в обработку, 400 — неверный формат запроса,
// 401 — пользователь не аутентифицирован, 409 — заказ загружен другим
// пользователем, 422 — неверный формат номера заказа,
// 500 — внутренняя ошибка.
func (h *Handler) UploadOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read request body", http.StatusBadRequest)
		return
	}
	number := strings.TrimSpace(string(body))
	if number == "" {
		http.Error(w, "order number must not be empty", http.StatusBadRequest)
		return
	}

	err = h.orders.Upload(r.Context(), userID, number)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusAccepted)
	case errors.Is(err, models.ErrOrderAlreadyUploaded):
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, models.ErrOrderOwnedByAnother):
		http.Error(w, "order already uploaded by another user", http.StatusConflict)
	case errors.Is(err, models.ErrInvalidOrderNumber):
		http.Error(w, "invalid order number format", http.StatusUnprocessableEntity)
	default:
		h.internalError(w, "upload order", err)
	}
}

// ListOrders обрабатывает GET /api/user/orders: отдаёт загруженные
// пользователем заказы, отсортированные от новых к старым.
//
// Коды ответа: 200 — успех, 204 — нет данных,
// 401 — пользователь не аутентифицирован, 500 — внутренняя ошибка.
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	orders, err := h.orders.List(r.Context(), userID)
	if err != nil {
		h.internalError(w, "list orders", err)
		return
	}
	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	h.writeJSON(w, http.StatusOK, orders)
}
