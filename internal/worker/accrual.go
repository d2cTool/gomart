// Package worker содержит фоновые обработчики системы лояльности.
// Основной из них опрашивает внешнюю систему расчёта баллов и обновляет
// статусы заказов вместе с накопительными счетами пользователей.
package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/d2cTool/gomart/internal/models"
)

// PendingOrderRepository — хранилище заказов, требующих опроса системы
// расчёта начислений.
type PendingOrderRepository interface {
	// TakePending забирает до limit заказов в неокончательных статусах.
	TakePending(ctx context.Context, limit int) ([]string, error)
	// ApplyAccrual сохраняет результат расчёта и начисляет баллы.
	ApplyAccrual(ctx context.Context, number string, status models.OrderStatus, accrual models.Points) error
}

// AccrualClient — клиент внешней системы расчёта баллов лояльности.
type AccrualClient interface {
	// OrderInfo возвращает информацию о расчёте начисления по заказу.
	OrderInfo(ctx context.Context, number string) (models.AccrualInfo, error)
}

// Options настраивают работу опросчика системы расчёта начислений.
type Options struct {
	// Interval — пауза между проходами по очереди заказов.
	Interval time.Duration
	// Workers — количество параллельных обработчиков заказов.
	Workers int
	// BatchSize — количество заказов, забираемых из хранилища за проход.
	BatchSize int
}

// AccrualPoller периодически опрашивает систему расчёта начислений
// по заказам в неокончательных статусах.
type AccrualPoller struct {
	orders  PendingOrderRepository
	client  AccrualClient
	log     *zap.Logger
	opts    Options
	limiter *limiter
}

// NewAccrualPoller создаёт опросчик системы расчёта начислений.
// Некорректные значения в opts заменяются разумными значениями по умолчанию.
func NewAccrualPoller(orders PendingOrderRepository, client AccrualClient, log *zap.Logger, opts Options) *AccrualPoller {
	if opts.Interval <= 0 {
		opts.Interval = time.Second
	}
	if opts.Workers <= 0 {
		opts.Workers = 1
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 10
	}
	return &AccrualPoller{
		orders:  orders,
		client:  client,
		log:     log,
		opts:    opts,
		limiter: &limiter{},
	}
}

// Run запускает цикл опроса и завершает его при отмене контекста.
func (p *AccrualPoller) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		if err := p.ProcessBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
			p.log.Error("process accrual batch", zap.Error(err))
		}
	}
}

// ProcessBatch забирает очередную пачку заказов и опрашивает по ним систему
// расчёта начислений. Метод экспортирован, чтобы обработку можно было
// вызывать точечно, в том числе из тестов.
func (p *AccrualPoller) ProcessBatch(ctx context.Context) error {
	numbers, err := p.orders.TakePending(ctx, p.opts.BatchSize)
	if err != nil {
		return err
	}
	if len(numbers) == 0 {
		return nil
	}

	jobs := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < p.opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for number := range jobs {
				p.processOrder(ctx, number)
			}
		}()
	}

	for _, number := range numbers {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case jobs <- number:
		}
	}
	close(jobs)
	wg.Wait()
	return nil
}

// processOrder опрашивает систему расчёта по одному заказу и сохраняет
// результат. Ошибки не прерывают обработку остальных заказов: заказ
// останется в очереди и будет опрошен на следующем проходе.
func (p *AccrualPoller) processOrder(ctx context.Context, number string) {
	if err := p.limiter.wait(ctx); err != nil {
		return
	}

	info, err := p.client.OrderInfo(ctx, number)
	if err != nil {
		p.handleClientError(number, err)
		return
	}

	var accrual models.Points
	if info.Accrual != nil {
		accrual = *info.Accrual
	}

	status := info.Status.OrderStatus()
	if err := p.orders.ApplyAccrual(ctx, number, status, accrual); err != nil {
		if !errors.Is(err, context.Canceled) {
			p.log.Error("apply accrual", zap.String("order", number), zap.Error(err))
		}
		return
	}

	p.log.Debug("order processed",
		zap.String("order", number),
		zap.String("status", string(status)),
		zap.String("accrual", accrual.String()),
	)
}

// handleClientError разбирает ошибку обращения к системе расчёта начислений.
func (p *AccrualPoller) handleClientError(number string, err error) {
	var tooMany *models.TooManyRequestsError
	switch {
	case errors.As(err, &tooMany):
		pause := time.Duration(tooMany.RetryAfterSeconds) * time.Second
		p.limiter.pause(pause)
		p.log.Warn("accrual system rate limit reached", zap.Duration("retry_after", pause))
	case errors.Is(err, models.ErrOrderNotRegistered):
		p.log.Debug("order is not registered in accrual system", zap.String("order", number))
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
	default:
		p.log.Error("request accrual info", zap.String("order", number), zap.Error(err))
	}
}

// limiter приостанавливает обращения к системе расчёта начислений после
// получения ответа 429 Too Many Requests.
type limiter struct {
	mu    sync.Mutex
	until time.Time
}

// pause откладывает следующие обращения на указанную длительность.
func (l *limiter) pause(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	deadline := time.Now().Add(d)
	if deadline.After(l.until) {
		l.until = deadline
	}
}

// wait блокирует вызывающую горутину до окончания паузы либо до отмены
// контекста.
func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	until := l.until
	l.mu.Unlock()

	delay := time.Until(until)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
