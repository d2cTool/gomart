package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	appmw "github.com/d2cTool/gomart/internal/middleware"
)

// NewRouter собирает маршрутизатор HTTP API системы лояльности.
// Публичными остаются только регистрация и аутентификация, остальные
// маршруты защищены проверкой токена.
func NewRouter(h *Handler, parser appmw.TokenParser, log *zap.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(appmw.Logging(log))
	r.Use(appmw.Gzip)

	r.Route("/api/user", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)

		r.Group(func(r chi.Router) {
			r.Use(appmw.Auth(parser))
			r.Post("/orders", h.UploadOrder)
			r.Get("/orders", h.ListOrders)
			r.Get("/balance", h.GetBalance)
			r.Post("/balance/withdraw", h.Withdraw)
			r.Get("/withdrawals", h.ListWithdrawals)
		})
	})

	return r
}
