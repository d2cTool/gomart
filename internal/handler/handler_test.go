package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/d2cTool/gomart/internal/handler"
	"github.com/d2cTool/gomart/internal/middleware"
	"github.com/d2cTool/gomart/internal/models"
)

// errInternal — искусственная ошибка сервисного слоя.
var errInternal = errors.New("something went wrong")

// authServiceStub — подставная реализация handler.AuthService.
type authServiceStub struct {
	token       string
	registerErr error
	loginErr    error
	gotLogin    string
	gotPassword string
}

func (s *authServiceStub) Register(_ context.Context, login, password string) (string, error) {
	s.gotLogin, s.gotPassword = login, password
	return s.token, s.registerErr
}

func (s *authServiceStub) Login(_ context.Context, login, password string) (string, error) {
	s.gotLogin, s.gotPassword = login, password
	return s.token, s.loginErr
}

// orderServiceStub — подставная реализация handler.OrderService.
type orderServiceStub struct {
	uploadErr error
	gotNumber string
	gotUserID int64
	orders    []models.Order
	listErr   error
}

func (s *orderServiceStub) Upload(_ context.Context, userID int64, number string) error {
	s.gotUserID, s.gotNumber = userID, number
	return s.uploadErr
}

func (s *orderServiceStub) List(context.Context, int64) ([]models.Order, error) {
	return s.orders, s.listErr
}

// balanceServiceStub — подставная реализация handler.BalanceService.
type balanceServiceStub struct {
	balance     models.Balance
	getErr      error
	withdrawErr error
	gotOrder    string
	gotSum      models.Points
	withdrawals []models.Withdrawal
	listErr     error
}

func (s *balanceServiceStub) Get(context.Context, int64) (models.Balance, error) {
	return s.balance, s.getErr
}

func (s *balanceServiceStub) Withdraw(_ context.Context, _ int64, order string, sum models.Points) error {
	s.gotOrder, s.gotSum = order, sum
	return s.withdrawErr
}

func (s *balanceServiceStub) Withdrawals(context.Context, int64) ([]models.Withdrawal, error) {
	return s.withdrawals, s.listErr
}

// parserStub — подставная реализация middleware.TokenParser.
type parserStub struct {
	userID int64
	err    error
}

func (s parserStub) Parse(string) (int64, error) { return s.userID, s.err }

// newTestServer поднимает маршрутизатор с подставными сервисами.
func newTestServer(t *testing.T, auth handler.AuthService, orders handler.OrderService, balances handler.BalanceService, parser middleware.TokenParser) *httptest.Server {
	t.Helper()
	log := zaptest.NewLogger(t)
	srv := httptest.NewServer(handler.NewRouter(handler.New(auth, orders, balances, log), parser, log))
	t.Cleanup(srv.Close)
	return srv
}

// do выполняет запрос к тестовому серверу с токеном аутентификации.
func do(t *testing.T, srv *httptest.Server, method, path, contentType, body string, authorize bool) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if authorize {
		req.Header.Set("Authorization", "Bearer token")
	}
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestRegister(t *testing.T) {
	auth := &authServiceStub{token: "issued-token"}
	srv := newTestServer(t, auth, &orderServiceStub{}, &balanceServiceStub{}, parserStub{userID: 1})

	resp := do(t, srv, http.MethodPost, "/api/user/register", "application/json", `{"login":"alice","password":"s3cret"}`, false)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Bearer issued-token", resp.Header.Get("Authorization"))
	assert.Equal(t, "alice", auth.gotLogin)
	assert.Equal(t, "s3cret", auth.gotPassword)

	var found bool
	for _, cookie := range resp.Cookies() {
		if cookie.Name == middleware.AuthCookieName {
			found = true
			assert.Equal(t, "issued-token", cookie.Value)
		}
	}
	assert.True(t, found, "auth cookie must be set")
}

func TestRegisterErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
	}{
		{name: "bad json", body: `{"login":`, wantStatus: http.StatusBadRequest},
		{name: "empty login", body: `{"login":"","password":"s3cret"}`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"login":"a","password":"b","role":"admin"}`, wantStatus: http.StatusBadRequest},
		{name: "login taken", body: `{"login":"alice","password":"s3cret"}`, err: models.ErrLoginTaken, wantStatus: http.StatusConflict},
		{name: "invalid credentials", body: `{"login":"alice","password":"s3cret"}`, err: models.ErrInvalidCredentials, wantStatus: http.StatusBadRequest},
		{name: "internal", body: `{"login":"alice","password":"s3cret"}`, err: errInternal, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, &authServiceStub{registerErr: tt.err}, &orderServiceStub{}, &balanceServiceStub{}, parserStub{})
			resp := do(t, srv, http.MethodPost, "/api/user/register", "application/json", tt.body, false)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestLogin(t *testing.T) {
	srv := newTestServer(t, &authServiceStub{token: "issued-token"}, &orderServiceStub{}, &balanceServiceStub{}, parserStub{})

	resp := do(t, srv, http.MethodPost, "/api/user/login", "application/json", `{"login":"alice","password":"s3cret"}`, false)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Bearer issued-token", resp.Header.Get("Authorization"))
}

func TestLoginErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
	}{
		{name: "bad json", body: `[]`, wantStatus: http.StatusBadRequest},
		{name: "wrong pair", body: `{"login":"alice","password":"s3cret"}`, err: models.ErrInvalidCredentials, wantStatus: http.StatusUnauthorized},
		{name: "internal", body: `{"login":"alice","password":"s3cret"}`, err: errInternal, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, &authServiceStub{loginErr: tt.err}, &orderServiceStub{}, &balanceServiceStub{}, parserStub{})
			resp := do(t, srv, http.MethodPost, "/api/user/login", "application/json", tt.body, false)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestUploadOrder(t *testing.T) {
	orders := &orderServiceStub{}
	srv := newTestServer(t, &authServiceStub{}, orders, &balanceServiceStub{}, parserStub{userID: 7})

	resp := do(t, srv, http.MethodPost, "/api/user/orders", "text/plain", "12345678903\n", true)

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.Equal(t, int64(7), orders.gotUserID)
	assert.Equal(t, "12345678903", orders.gotNumber)
}

func TestUploadOrderStatuses(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
	}{
		{name: "already uploaded by user", body: "12345678903", err: models.ErrOrderAlreadyUploaded, wantStatus: http.StatusOK},
		{name: "uploaded by another user", body: "12345678903", err: models.ErrOrderOwnedByAnother, wantStatus: http.StatusConflict},
		{name: "invalid number", body: "12345678901", err: models.ErrInvalidOrderNumber, wantStatus: http.StatusUnprocessableEntity},
		{name: "empty body", body: "  ", wantStatus: http.StatusBadRequest},
		{name: "internal", body: "12345678903", err: errInternal, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, &authServiceStub{}, &orderServiceStub{uploadErr: tt.err}, &balanceServiceStub{}, parserStub{userID: 7})
			resp := do(t, srv, http.MethodPost, "/api/user/orders", "text/plain", tt.body, true)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestProtectedEndpointsRequireAuth(t *testing.T) {
	srv := newTestServer(t, &authServiceStub{}, &orderServiceStub{}, &balanceServiceStub{}, parserStub{err: errors.New("invalid")})

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/user/orders"},
		{http.MethodGet, "/api/user/orders"},
		{http.MethodGet, "/api/user/balance"},
		{http.MethodPost, "/api/user/balance/withdraw"},
		{http.MethodGet, "/api/user/withdrawals"},
	}

	for _, p := range paths {
		t.Run(p.method+" "+p.path, func(t *testing.T) {
			resp := do(t, srv, p.method, p.path, "", "", false)
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})
	}
}

func TestListOrders(t *testing.T) {
	accrual := models.NewPoints(500)
	orders := &orderServiceStub{orders: []models.Order{
		{Number: "9278923470", Status: models.StatusProcessed, Accrual: &accrual, UploadedAt: time.Now()},
		{Number: "12345678903", Status: models.StatusProcessing, UploadedAt: time.Now().Add(-time.Hour)},
	}}
	srv := newTestServer(t, &authServiceStub{}, orders, &balanceServiceStub{}, parserStub{userID: 7})

	resp := do(t, srv, http.MethodGet, "/api/user/orders", "", "", true)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var got []models.Order
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 2)
	assert.Equal(t, "9278923470", got[0].Number)
	require.NotNil(t, got[0].Accrual)
	assert.Equal(t, models.NewPoints(500), *got[0].Accrual)
	assert.Nil(t, got[1].Accrual)
}

func TestListOrdersNoContent(t *testing.T) {
	srv := newTestServer(t, &authServiceStub{}, &orderServiceStub{}, &balanceServiceStub{}, parserStub{userID: 7})

	resp := do(t, srv, http.MethodGet, "/api/user/orders", "", "", true)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestListOrdersInternalError(t *testing.T) {
	srv := newTestServer(t, &authServiceStub{}, &orderServiceStub{listErr: errInternal}, &balanceServiceStub{}, parserStub{userID: 7})

	resp := do(t, srv, http.MethodGet, "/api/user/orders", "", "", true)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestGetBalance(t *testing.T) {
	balances := &balanceServiceStub{balance: models.Balance{Current: models.NewPoints(500.5), Withdrawn: models.NewPoints(42)}}
	srv := newTestServer(t, &authServiceStub{}, &orderServiceStub{}, balances, parserStub{userID: 7})

	resp := do(t, srv, http.MethodGet, "/api/user/balance", "", "", true)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got models.Balance
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, models.NewPoints(500.5), got.Current)
	assert.Equal(t, models.NewPoints(42), got.Withdrawn)
}

func TestGetBalanceInternalError(t *testing.T) {
	srv := newTestServer(t, &authServiceStub{}, &orderServiceStub{}, &balanceServiceStub{getErr: errInternal}, parserStub{userID: 7})

	resp := do(t, srv, http.MethodGet, "/api/user/balance", "", "", true)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestWithdraw(t *testing.T) {
	balances := &balanceServiceStub{}
	srv := newTestServer(t, &authServiceStub{}, &orderServiceStub{}, balances, parserStub{userID: 7})

	resp := do(t, srv, http.MethodPost, "/api/user/balance/withdraw", "application/json", `{"order":"2377225624","sum":751}`, true)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "2377225624", balances.gotOrder)
	assert.Equal(t, models.NewPoints(751), balances.gotSum)
}

func TestWithdrawErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
	}{
		{name: "bad json", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "insufficient funds", body: `{"order":"2377225624","sum":751}`, err: models.ErrInsufficientFunds, wantStatus: http.StatusPaymentRequired},
		{name: "invalid order", body: `{"order":"123","sum":751}`, err: models.ErrInvalidOrderNumber, wantStatus: http.StatusUnprocessableEntity},
		{name: "duplicate withdrawal", body: `{"order":"2377225624","sum":751}`, err: models.ErrWithdrawalExists, wantStatus: http.StatusUnprocessableEntity},
		{name: "internal", body: `{"order":"2377225624","sum":751}`, err: errInternal, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, &authServiceStub{}, &orderServiceStub{}, &balanceServiceStub{withdrawErr: tt.err}, parserStub{userID: 7})
			resp := do(t, srv, http.MethodPost, "/api/user/balance/withdraw", "application/json", tt.body, true)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestListWithdrawals(t *testing.T) {
	balances := &balanceServiceStub{withdrawals: []models.Withdrawal{
		{Order: "2377225624", Sum: models.NewPoints(500), ProcessedAt: time.Now()},
	}}
	srv := newTestServer(t, &authServiceStub{}, &orderServiceStub{}, balances, parserStub{userID: 7})

	resp := do(t, srv, http.MethodGet, "/api/user/withdrawals", "", "", true)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []models.Withdrawal
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 1)
	assert.Equal(t, "2377225624", got[0].Order)
	assert.Equal(t, models.NewPoints(500), got[0].Sum)
}

func TestListWithdrawalsNoContent(t *testing.T) {
	srv := newTestServer(t, &authServiceStub{}, &orderServiceStub{}, &balanceServiceStub{}, parserStub{userID: 7})

	resp := do(t, srv, http.MethodGet, "/api/user/withdrawals", "", "", true)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestListWithdrawalsInternalError(t *testing.T) {
	srv := newTestServer(t, &authServiceStub{}, &orderServiceStub{}, &balanceServiceStub{listErr: errInternal}, parserStub{userID: 7})

	resp := do(t, srv, http.MethodGet, "/api/user/withdrawals", "", "", true)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
