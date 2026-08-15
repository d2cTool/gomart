// Command gophermart запускает HTTP-сервис накопительной системы лояльности
// «Гофермарт».
//
// Параметры запуска задаются флагами командной строки или переменными
// окружения:
//
//	-a, RUN_ADDRESS            — адрес и порт запуска сервиса;
//	-d, DATABASE_URI           — адрес подключения к PostgreSQL;
//	-r, ACCRUAL_SYSTEM_ADDRESS — адрес системы расчёта начислений.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/d2cTool/gomart/internal/app"
	"github.com/d2cTool/gomart/internal/config"
	"github.com/d2cTool/gomart/internal/logger"
)

func main() {
	if err := run(); err != nil {
		fail(err)
	}
}

// run выполняет полный цикл работы сервиса: разбирает конфигурацию,
// поднимает логгер и запускает приложение до получения сигнала завершения.
func run() error {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		return err
	}

	log, err := logger.New(cfg.LogLevel)
	if err != nil {
		return err
	}
	defer func() { _ = log.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	return app.Run(ctx, cfg, log)
}

// fail печатает причину аварийного завершения и выходит с ненулевым кодом.
func fail(err error) {
	fmt.Fprintf(os.Stderr, "gophermart: %v\n", err)
	os.Exit(1)
}
