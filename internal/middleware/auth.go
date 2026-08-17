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
// токеном аутентификации. Источник токена выбирается по правилам
// tokenFromRequest. Идентификатор пользователя помещается в контекст запроса.
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

// tokenFromRequest извлекает токен аутентификации.
//
// Источники в порядке приоритета:
//  1. заголовок Authorization — схема Bearer или само значение заголовка;
//  2. cookie AuthCookieName.
//
// Cookie читается только если в заголовке нет токена: заголовок отсутствует,
// пуст или содержит одну схему Bearer без учётных данных. Если заголовок
// задаёт непустое значение, cookie не используется, даже если это значение
// не проходит проверку подписи.
func tokenFromRequest(r *http.Request) string {
	if token := tokenFromAuthorization(r.Header.Get("Authorization")); token != "" {
		return token
	}
	cookie, err := r.Cookie(AuthCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// tokenFromAuthorization извлекает токен из значения заголовка Authorization.
// Пустая строка означает, что в заголовке токена нет и можно обратиться
// к следующему источнику.
func tokenFromAuthorization(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}

	const scheme = "Bearer "
	if len(header) >= len(scheme) && strings.EqualFold(header[:len(scheme)], scheme) {
		return strings.TrimSpace(header[len(scheme):])
	}
	if strings.EqualFold(header, strings.TrimSpace(scheme)) {
		return ""
	}
	return header
}
