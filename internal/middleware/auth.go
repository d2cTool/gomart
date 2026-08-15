// Package middleware содержит HTTP-посредники сервиса: аутентификацию,
// прозрачное сжатие данных и логирование запросов.
package middleware

import (
	"context"
	"net/http"
	"strings"
)

// AuthCookieName — имя cookie, в которой дублируется токен аутентификации.
const AuthCookieName = "gophermart-auth"

// contextKey — тип ключей значений, помещаемых посредниками в контекст запроса.
type contextKey struct{ name string }

// userIDKey — ключ идентификатора аутентифицированного пользователя.
var userIDKey = contextKey{name: "user-id"}

// TokenParser проверяет токен аутентификации и возвращает идентификатор
// пользователя, которому он выдан.
type TokenParser interface {
	// Parse проверяет токен и возвращает идентификатор пользователя.
	Parse(token string) (int64, error)
}

// Auth возвращает посредник, пропускающий дальше только запросы с корректным
// токеном аутентификации. Токен читается из заголовка Authorization
// (схема Bearer) либо из cookie. Идентификатор пользователя помещается
// в контекст запроса.
func Auth(parser TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := tokenFromRequest(r)
			if token == "" {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			userID, err := parser.Parse(token)
			if err != nil {
				http.Error(w, "invalid authentication token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
		})
	}
}

// WithUserID возвращает контекст с идентификатором аутентифицированного
// пользователя.
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext извлекает идентификатор пользователя из контекста запроса.
// Второе возвращаемое значение сообщает, был ли пользователь аутентифицирован.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userIDKey).(int64)
	return userID, ok
}

// tokenFromRequest достаёт токен из заголовка Authorization или из cookie.
func tokenFromRequest(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header != "" {
		const scheme = "Bearer "
		if len(header) > len(scheme) && strings.EqualFold(header[:len(scheme)], scheme) {
			return strings.TrimSpace(header[len(scheme):])
		}
		return header
	}
	if cookie, err := r.Cookie(AuthCookieName); err == nil {
		return cookie.Value
	}
	return ""
}
