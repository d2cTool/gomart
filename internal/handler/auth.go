package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/d2cTool/gomart/internal/middleware"
	"github.com/d2cTool/gomart/internal/models"
)

// credentials — тело запросов регистрации и аутентификации.
type credentials struct {
	// Login — логин пользователя.
	Login string `json:"login"`
	// Password — пароль пользователя.
	Password string `json:"password"`
}

// Register обрабатывает POST /api/user/register: регистрирует пользователя
// и сразу его аутентифицирует.
//
// Коды ответа: 200 — успех, 400 — неверный формат запроса,
// 409 — логин занят, 500 — внутренняя ошибка.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	creds, ok := decodeCredentials(w, r)
	if !ok {
		return
	}

	token, err := h.auth.Register(r.Context(), creds.Login, creds.Password)
	switch {
	case err == nil:
		writeAuthToken(w, token)
	case errors.Is(err, models.ErrLoginTaken):
		http.Error(w, "login is already taken", http.StatusConflict)
	case errors.Is(err, models.ErrInvalidCredentials):
		http.Error(w, "login and password must not be empty", http.StatusBadRequest)
	default:
		h.internalError(w, "register user", err)
	}
}

// Login обрабатывает POST /api/user/login: аутентифицирует пользователя
// по паре логин/пароль.
//
// Коды ответа: 200 — успех, 400 — неверный формат запроса,
// 401 — неверная пара логин/пароль, 500 — внутренняя ошибка.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	creds, ok := decodeCredentials(w, r)
	if !ok {
		return
	}

	token, err := h.auth.Login(r.Context(), creds.Login, creds.Password)
	switch {
	case err == nil:
		writeAuthToken(w, token)
	case errors.Is(err, models.ErrInvalidCredentials):
		http.Error(w, "invalid login/password pair", http.StatusUnauthorized)
	default:
		h.internalError(w, "login user", err)
	}
}

// decodeCredentials разбирает тело запроса с парой логин/пароль.
// При некорректном теле клиенту отдаётся 400.
func decodeCredentials(w http.ResponseWriter, r *http.Request) (credentials, bool) {
	var creds credentials
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&creds); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return credentials{}, false
	}
	if creds.Login == "" || creds.Password == "" {
		http.Error(w, "login and password must not be empty", http.StatusBadRequest)
		return credentials{}, false
	}
	return creds, true
}

// writeAuthToken передаёт клиенту токен аутентификации заголовком Authorization,
// дублирует его в cookie и отвечает кодом 200.
func writeAuthToken(w http.ResponseWriter, token string) {
	w.Header().Set("Authorization", "Bearer "+token)
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AuthCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusOK)
}
