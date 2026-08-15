package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/d2cTool/gomart/internal/models"
	"github.com/d2cTool/gomart/internal/worker"
)

// appliedAccrual — зафиксированный результат обработки заказа.
type appliedAccrual struct {
	number  string
	status  models.OrderStatus
	accrual models.Points
}

// orderRepoStub — подставная реализация worker.PendingOrderRepository.
type orderRepoStub struct {
	mu       sync.Mutex
	batches  [][]string
	takeErr  error
	applied  []appliedAccrual
	applyErr error
}

func (s *orderRepoStub) TakePending(context.Context, int) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.takeErr != nil {
		return nil, s.takeErr
	}
	if len(s.batches) == 0 {
		return nil, nil
	}
	batch := s.batches[0]
	s.batches = s.batches[1:]
	return batch, nil
}

func (s *orderRepoStub) ApplyAccrual(_ context.Context, number string, status models.OrderStatus, accrual models.Points) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.applyErr != nil {
		return s.applyErr
	}
	s.applied = append(s.applied, appliedAccrual{number: number, status: status, accrual: accrual})
	return nil
}

func (s *orderRepoStub) appliedOrders() []appliedAccrual {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]appliedAccrual(nil), s.applied...)
}

// accrualClientStub — подставная реализация worker.AccrualClient.
type accrualClientStub struct {
	mu        sync.Mutex
	responses map[string]models.AccrualInfo
	errs      map[string]error
	requests  []string
}

func (s *accrualClientStub) OrderInfo(_ context.Context, number string) (models.AccrualInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, number)
	if err, ok := s.errs[number]; ok {
		return models.AccrualInfo{}, err
	}
	return s.responses[number], nil
}

func (s *accrualClientStub) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

// testOptions — настройки опросчика, ускоренные для тестов.
var testOptions = worker.Options{Interval: 10 * time.Millisecond, Workers: 2, BatchSize: 10}

func TestProcessBatchAppliesAccrual(t *testing.T) {
	accrual := models.NewPoints(500.5)
	orders := &orderRepoStub{batches: [][]string{{"12345678903", "9278923470", "346436439"}}}
	client := &accrualClientStub{responses: map[string]models.AccrualInfo{
		"12345678903": {Order: "12345678903", Status: models.AccrualProcessed, Accrual: &accrual},
		"9278923470":  {Order: "9278923470", Status: models.AccrualProcessing},
		"346436439":   {Order: "346436439", Status: models.AccrualInvalid},
	}}
	poller := worker.NewAccrualPoller(orders, client, zaptest.NewLogger(t), testOptions)

	require.NoError(t, poller.ProcessBatch(context.Background()))

	applied := orders.appliedOrders()
	require.Len(t, applied, 3)

	byNumber := make(map[string]appliedAccrual, len(applied))
	for _, a := range applied {
		byNumber[a.number] = a
	}
	assert.Equal(t, models.StatusProcessed, byNumber["12345678903"].status)
	assert.Equal(t, models.NewPoints(500.5), byNumber["12345678903"].accrual)
	assert.Equal(t, models.StatusProcessing, byNumber["9278923470"].status)
	assert.Equal(t, models.Points(0), byNumber["9278923470"].accrual)
	assert.Equal(t, models.StatusInvalid, byNumber["346436439"].status)
}

func TestProcessBatchEmptyQueue(t *testing.T) {
	orders := &orderRepoStub{}
	poller := worker.NewAccrualPoller(orders, &accrualClientStub{}, zaptest.NewLogger(t), testOptions)

	require.NoError(t, poller.ProcessBatch(context.Background()))
	assert.Empty(t, orders.appliedOrders())
}

func TestProcessBatchTakeError(t *testing.T) {
	wantErr := errors.New("database is down")
	poller := worker.NewAccrualPoller(&orderRepoStub{takeErr: wantErr}, &accrualClientStub{}, zaptest.NewLogger(t), testOptions)

	assert.ErrorIs(t, poller.ProcessBatch(context.Background()), wantErr)
}

func TestProcessBatchSkipsUnregisteredOrders(t *testing.T) {
	orders := &orderRepoStub{batches: [][]string{{"12345678903"}}}
	client := &accrualClientStub{errs: map[string]error{"12345678903": models.ErrOrderNotRegistered}}
	poller := worker.NewAccrualPoller(orders, client, zaptest.NewLogger(t), testOptions)

	require.NoError(t, poller.ProcessBatch(context.Background()))
	assert.Empty(t, orders.appliedOrders())
}

func TestProcessBatchSurvivesApplyError(t *testing.T) {
	accrual := models.NewPoints(10)
	orders := &orderRepoStub{batches: [][]string{{"12345678903"}}, applyErr: errors.New("database is down")}
	client := &accrualClientStub{responses: map[string]models.AccrualInfo{
		"12345678903": {Status: models.AccrualProcessed, Accrual: &accrual},
	}}
	poller := worker.NewAccrualPoller(orders, client, zaptest.NewLogger(t), testOptions)

	assert.NoError(t, poller.ProcessBatch(context.Background()))
}

func TestProcessBatchPausesOnTooManyRequests(t *testing.T) {
	orders := &orderRepoStub{batches: [][]string{{"12345678903", "9278923470"}}}
	client := &accrualClientStub{errs: map[string]error{
		"12345678903": &models.TooManyRequestsError{RetryAfterSeconds: 60},
		"9278923470":  &models.TooManyRequestsError{RetryAfterSeconds: 60},
	}}
	poller := worker.NewAccrualPoller(orders, client, zaptest.NewLogger(t), worker.Options{
		Interval: 10 * time.Millisecond, Workers: 1, BatchSize: 10,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	require.NoError(t, poller.ProcessBatch(ctx))

	assert.Less(t, time.Since(start), time.Second, "паузу нельзя выдерживать дольше отмены контекста")
	assert.Equal(t, 1, client.requestCount(), "после 429 следующий заказ не должен опрашиваться сразу")
	assert.Empty(t, orders.appliedOrders())
}

func TestProcessBatchStopsOnCanceledContext(t *testing.T) {
	orders := &orderRepoStub{batches: [][]string{{"12345678903"}}}
	poller := worker.NewAccrualPoller(orders, &accrualClientStub{}, zaptest.NewLogger(t), testOptions)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.ErrorIs(t, poller.ProcessBatch(ctx), context.Canceled)
}

func TestRunStopsOnContextCancel(t *testing.T) {
	accrual := models.NewPoints(100)
	orders := &orderRepoStub{batches: [][]string{{"12345678903"}}}
	client := &accrualClientStub{responses: map[string]models.AccrualInfo{
		"12345678903": {Status: models.AccrualProcessed, Accrual: &accrual},
	}}
	poller := worker.NewAccrualPoller(orders, client, zaptest.NewLogger(t), testOptions)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- poller.Run(ctx) }()

	assert.Eventually(t, func() bool { return len(orders.appliedOrders()) == 1 }, time.Second, 10*time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("поллер не завершился после отмены контекста")
	}
}

func TestNewAccrualPollerNormalizesOptions(t *testing.T) {
	orders := &orderRepoStub{batches: [][]string{{"12345678903"}}}
	client := &accrualClientStub{responses: map[string]models.AccrualInfo{
		"12345678903": {Status: models.AccrualInvalid},
	}}
	poller := worker.NewAccrualPoller(orders, client, zaptest.NewLogger(t), worker.Options{})

	require.NoError(t, poller.ProcessBatch(context.Background()))
	require.Len(t, orders.appliedOrders(), 1)
}
