//go:build integration

// Сквозной интеграционный тест сервиса: поднимает приложение с реальной базой
// данных и подставной системой расчёта начислений. Требует переменной
// окружения TEST_DATABASE_URI:
//
//	go test -tags=integration ./internal/app/...
package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/d2cTool/gomart/internal/app"
	"github.com/d2cTool/gomart/internal/config"
	"github.com/d2cTool/gomart/internal/models"
	"github.com/d2cTool/gomart/internal/repository/postgres"
)

// testOrder — номер заказа, проходящий проверку по алгоритму Луна.
const testOrder = "12345678903"

// testSchema — отдельная схема БД, чтобы сквозной тест не мешал тестам
// хранилища, работающим с той же базой.
const testSchema = "test_app"

// schemaDSN добавляет к строке подключения переключение на указанную схему.
func schemaDSN(t *testing.T, base, schema string) string {
	t.Helper()
	u, err := url.Parse(base)
	require.NoError(t, err)
	q := u.Query()
	q.Set("options", "-c search_path="+schema)
	u.RawQuery = q.Encode()
	return u.String()
}

// freeAddress подбирает свободный TCP-адрес на локальном интерфейсе.
func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	return listener.Addr().String()
}

// prepareDatabase создаёт тестовую схему и приводит её в исходное состояние,
// возвращая строку подключения к этой схеме.
func prepareDatabase(t *testing.T, base string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := postgres.NewPool(ctx, base)
	require.NoError(t, err)
	_, err = admin.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+testSchema)
	admin.Close()
	require.NoError(t, err)

	dsn := schemaDSN(t, base, testSchema)
	pool, err := postgres.NewPool(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	require.NoError(t, postgres.Migrate(ctx, pool))
	_, err = pool.Exec(ctx, `TRUNCATE withdrawals, orders, balances, users RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
	return dsn
}

// waitForServer дожидается готовности HTTP-сервера принимать соединения.
func waitForServer(t *testing.T, address string) {
	t.Helper()
	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 10*time.Second, 50*time.Millisecond, "сервис не поднялся")
}

func TestGophermartEndToEnd(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URI")
	if base == "" {
		t.Skip("TEST_DATABASE_URI is not set, skipping integration test")
	}
	dsn := prepareDatabase(t, base)

	accrualSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/orders/"+testOrder {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"order":%q,"status":"PROCESSED","accrual":500}`, testOrder)
	}))
	defer accrualSrv.Close()

	address := freeAddress(t)
	cfg := &config.Config{
		RunAddress:           address,
		DatabaseURI:          dsn,
		AccrualSystemAddress: accrualSrv.URL,
		JWTSecret:            "integration-secret",
		TokenTTL:             time.Hour,
		AccrualPollInterval:  50 * time.Millisecond,
		AccrualWorkers:       2,
		AccrualBatchSize:     10,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, cfg, zaptest.NewLogger(t)) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(15 * time.Second):
			t.Error("сервис не завершился после отмены контекста")
		}
	}()

	waitForServer(t, address)
	baseURL := "http://" + address
	client := &http.Client{Timeout: 5 * time.Second}

	// Регистрация пользователя.
	resp, err := client.Post(baseURL+"/api/user/register", "application/json",
		strings.NewReader(`{"login":"alice","password":"s3cret"}`))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)

	token := resp.Header.Get("Authorization")
	require.NotEmpty(t, token)

	// Вспомогательная функция запроса с аутентификацией.
	call := func(method, path, contentType, body string) *http.Response {
		req, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", token)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		return resp
	}

	// Загрузка номера заказа.
	resp = call(http.MethodPost, "/api/user/orders", "text/plain", testOrder)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	resp = call(http.MethodPost, "/api/user/orders", "text/plain", testOrder)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode, "повторная загрузка тем же пользователем")

	resp = call(http.MethodPost, "/api/user/orders", "text/plain", "12345678901")
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "номер не проходит проверку Луна")

	// Ожидание начисления баллов фоновым воркером.
	var balance models.Balance
	require.Eventually(t, func() bool {
		resp := call(http.MethodGet, "/api/user/balance", "", "")
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		if err := json.NewDecoder(resp.Body).Decode(&balance); err != nil {
			return false
		}
		return balance.Current == models.NewPoints(500)
	}, 15*time.Second, 100*time.Millisecond, "баллы не начислены")

	// Статус заказа обновлён.
	resp = call(http.MethodGet, "/api/user/orders", "", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var orders []models.Order
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&orders))
	require.NoError(t, resp.Body.Close())
	require.Len(t, orders, 1)
	assert.Equal(t, models.StatusProcessed, orders[0].Status)
	require.NotNil(t, orders[0].Accrual)
	assert.Equal(t, models.NewPoints(500), *orders[0].Accrual)

	// Списание баллов.
	resp = call(http.MethodPost, "/api/user/balance/withdraw", "application/json", `{"order":"2377225624","sum":100}`)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp = call(http.MethodPost, "/api/user/balance/withdraw", "application/json", `{"order":"9278923470","sum":100000}`)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusPaymentRequired, resp.StatusCode)

	resp = call(http.MethodGet, "/api/user/balance", "", "")
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&balance))
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, models.NewPoints(400), balance.Current)
	assert.Equal(t, models.NewPoints(100), balance.Withdrawn)

	// История списаний.
	resp = call(http.MethodGet, "/api/user/withdrawals", "", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var withdrawals []models.Withdrawal
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&withdrawals))
	require.NoError(t, resp.Body.Close())
	require.Len(t, withdrawals, 1)
	assert.Equal(t, "2377225624", withdrawals[0].Order)
	assert.Equal(t, models.NewPoints(100), withdrawals[0].Sum)

	// Неаутентифицированный доступ запрещён.
	resp, err = client.Get(baseURL + "/api/user/orders")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
