package accrual_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/d2cTool/gomart/internal/accrual"
	"github.com/d2cTool/gomart/internal/models"
)

func TestOrderInfoProcessed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/orders/12345678903", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"order":"12345678903","status":"PROCESSED","accrual":500.5}`))
	}))
	defer srv.Close()

	info, err := accrual.New(srv.URL).OrderInfo(context.Background(), "12345678903")
	require.NoError(t, err)
	assert.Equal(t, "12345678903", info.Order)
	assert.Equal(t, models.AccrualProcessed, info.Status)
	require.NotNil(t, info.Accrual)
	assert.Equal(t, models.NewPoints(500.5), *info.Accrual)
}

func TestOrderInfoWithoutAccrual(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"order":"12345678903","status":"REGISTERED"}`))
	}))
	defer srv.Close()

	info, err := accrual.New(srv.URL).OrderInfo(context.Background(), "12345678903")
	require.NoError(t, err)
	assert.Equal(t, models.AccrualRegistered, info.Status)
	assert.Nil(t, info.Accrual)
}

func TestOrderInfoNotRegistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	_, err := accrual.New(srv.URL).OrderInfo(context.Background(), "12345678903")
	assert.ErrorIs(t, err, models.ErrOrderNotRegistered)
}

func TestOrderInfoTooManyRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "13")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("No more than 60 requests per minute allowed"))
	}))
	defer srv.Close()

	_, err := accrual.New(srv.URL).OrderInfo(context.Background(), "12345678903")

	var tooMany *models.TooManyRequestsError
	require.ErrorAs(t, err, &tooMany)
	assert.Equal(t, 13, tooMany.RetryAfterSeconds)
	assert.Equal(t, "No more than 60 requests per minute allowed", tooMany.Message)
}

func TestOrderInfoTooManyRequestsWithoutHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := accrual.New(srv.URL).OrderInfo(context.Background(), "12345678903")

	var tooMany *models.TooManyRequestsError
	require.ErrorAs(t, err, &tooMany)
	assert.Equal(t, 60, tooMany.RetryAfterSeconds)
}

func TestOrderInfoServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := accrual.New(srv.URL).OrderInfo(context.Background(), "12345678903")
	assert.ErrorContains(t, err, "status 500")
}

func TestOrderInfoBrokenJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"order":`))
	}))
	defer srv.Close()

	_, err := accrual.New(srv.URL).OrderInfo(context.Background(), "12345678903")
	assert.ErrorContains(t, err, "decode accrual response")
}

func TestOrderInfoUnreadableBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "64")
		_, _ = w.Write([]byte(`{"order":"12345678903"`))
	}))
	defer srv.Close()

	_, err := accrual.New(srv.URL).OrderInfo(context.Background(), "12345678903")
	assert.ErrorContains(t, err, "read accrual response")
}

func TestOrderInfoUnreachableService(t *testing.T) {
	client := accrual.NewWithClient("http://127.0.0.1:1", &http.Client{Timeout: 100 * time.Millisecond})

	_, err := client.OrderInfo(context.Background(), "12345678903")
	assert.ErrorContains(t, err, "call accrual system")
}

func TestOrderInfoCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := accrual.New("http://127.0.0.1:1").OrderInfo(ctx, "12345678903")
	assert.ErrorIs(t, err, context.Canceled)
}
