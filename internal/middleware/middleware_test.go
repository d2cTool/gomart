package middleware_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/d2cTool/gomart/internal/middleware"
)

// parserStub — подставная реализация middleware.TokenParser.
type parserStub struct {
	userID int64
	err    error
}

func (s parserStub) Parse(string) (int64, error) { return s.userID, s.err }

// recordingParser запоминает токен, с которым его вызвали.
type recordingParser struct {
	userID int64
	got    string
}

func (s *recordingParser) Parse(token string) (int64, error) {
	s.got = token
	return s.userID, nil
}

// userIDHandler отдаёт идентификатор пользователя, найденный в контексте.
func userIDHandler(t *testing.T, want int64) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.UserIDFromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, want, userID)
		w.WriteHeader(http.StatusOK)
	})
}

func TestAuthWithBearerHeader(t *testing.T) {
	handler := middleware.Auth(parserStub{userID: 7})(userIDHandler(t, 7))

	req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthWithRawHeaderAndCookie(t *testing.T) {
	handler := middleware.Auth(parserStub{userID: 42})(userIDHandler(t, 42))

	req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	req.Header.Set("Authorization", "token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	req.AddCookie(&http.Cookie{Name: middleware.AuthCookieName, Value: "token"})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthPrefersAuthorizationOverCookie(t *testing.T) {
	parser := &recordingParser{userID: 7}
	handler := middleware.Auth(parser)(userIDHandler(t, 7))

	req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	req.Header.Set("Authorization", "Bearer from-header")
	req.AddCookie(&http.Cookie{Name: middleware.AuthCookieName, Value: "from-cookie"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "from-header", parser.got)
}

func TestAuthFallsBackToCookieWhenHeaderHasNoToken(t *testing.T) {
	tests := []string{"", "Bearer", "Bearer ", "Bearer   "}

	for _, header := range tests {
		t.Run("header="+header, func(t *testing.T) {
			parser := &recordingParser{userID: 7}
			handler := middleware.Auth(parser)(userIDHandler(t, 7))

			req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			req.AddCookie(&http.Cookie{Name: middleware.AuthCookieName, Value: "from-cookie"})
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "from-cookie", parser.got)
		})
	}
}

func TestAuthDoesNotFallBackToCookieWhenHeaderTokenIsPresent(t *testing.T) {
	handler := middleware.Auth(parserStub{err: errors.New("invalid")})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	req.Header.Set("Authorization", "Bearer broken")
	req.AddCookie(&http.Cookie{Name: middleware.AuthCookieName, Value: "valid-cookie"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthWithoutToken(t *testing.T) {
	handler := middleware.Auth(parserStub{userID: 7})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/user/orders", nil))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthWithInvalidToken(t *testing.T) {
	handler := middleware.Auth(parserStub{err: errors.New("invalid")})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
	req.Header.Set("Authorization", "Bearer broken")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestUserIDFromContextWithoutValue(t *testing.T) {
	_, ok := middleware.UserIDFromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context())
	assert.False(t, ok)
}

func TestGzipCompressesResponse(t *testing.T) {
	handler := middleware.Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"current":500.5}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	reader, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"current":500.5}`, string(body))
}

func TestGzipSkipsNonCompressibleContent(t *testing.T) {
	handler := middleware.Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50})
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, []byte{0x89, 0x50}, rec.Body.Bytes())
}

func TestGzipDecompressesRequest(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte("12345678903"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	handler := middleware.Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "12345678903", string(body))
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/user/orders", &buf)
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestGzipRejectsBrokenRequestBody(t *testing.T) {
	handler := middleware.Gzip(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/user/orders", strings.NewReader("not gzip"))
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLogging(t *testing.T) {
	log := zaptest.NewLogger(t)
	handler := middleware.Logging(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("body"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/user/orders", nil))

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "body", rec.Body.String())
}
