// Package app собирает компоненты сервиса накопительной системы лояльности
// в единое приложение и управляет его жизненным циклом.
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/d2cTool/gomart/internal/accrual"
	"github.com/d2cTool/gomart/internal/auth"
	"github.com/d2cTool/gomart/internal/config"
	"github.com/d2cTool/gomart/internal/handler"
	"github.com/d2cTool/gomart/internal/repository/postgres"
	"github.com/d2cTool/gomart/internal/service"
	"github.com/d2cTool/gomart/internal/worker"
)

// shutdownTimeout — время, отводимое на корректное завершение HTTP-сервера.
const shutdownTimeout = 10 * time.Second

// Run поднимает сервис: применяет миграции, запускает HTTP-сервер и фоновый
// опросчик системы расчёта начислений. Функция завершается после отмены
// контекста и корректной остановки всех компонентов.
func Run(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURI)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	users := postgres.NewUserRepository(pool)
	orders := postgres.NewOrderRepository(pool)
	balances := postgres.NewBalanceRepository(pool)

	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.TokenTTL)
	authService := service.NewAuthService(users, tokens)
	orderService := service.NewOrderService(orders)
	balanceService := service.NewBalanceService(balances)

	router := handler.New(authService, orderService, balanceService, tokens, log).Routes()

	srv := &http.Server{
		Addr:              cfg.RunAddress,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		log.Info("http server started", zap.String("address", cfg.RunAddress))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	group.Go(func() error {
		<-groupCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(groupCtx), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown http server: %w", err)
		}
		log.Info("http server stopped")
		return nil
	})

	if cfg.AccrualSystemAddress == "" {
		log.Warn("accrual system address is not configured, orders will stay in NEW status")
	} else {
		poller := worker.NewAccrualPoller(orders, accrual.New(cfg.AccrualSystemAddress), log, worker.Options{
			Interval:  cfg.AccrualPollInterval,
			Workers:   cfg.AccrualWorkers,
			BatchSize: cfg.AccrualBatchSize,
		})
		group.Go(func() error {
			log.Info("accrual poller started", zap.String("address", cfg.AccrualSystemAddress))
			return poller.Run(groupCtx)
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}
	return nil
}
