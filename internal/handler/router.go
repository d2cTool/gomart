package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	appmw "github.com/d2cTool/gomart/internal/middleware"
)

// Routes собирает маршрутизатор HTTP API системы лояльности.
// Публичными остаются только регистрация и аутентификация, остальные
// маршруты защищены проверкой токена.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(appmw.Logging(h.log))
	r.Use(appmw.Gzip)

	r.Route("/api/user", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)

		r.Group(func(r chi.Router) {
			r.Use(appmw.Auth(h.parser))
			r.Post("/orders", h.UploadOrder)
			r.Get("/orders", h.ListOrders)
			r.Get("/balance", h.GetBalance)
			r.Post("/balance/withdraw", h.Withdraw)
			r.Get("/withdrawals", h.ListWithdrawals)
		})
	})

	return r
}
