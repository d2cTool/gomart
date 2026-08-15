// Package config отвечает за конфигурирование сервиса накопительной системы
// лояльности «Гофермарт».
//
// Значения читаются из флагов командной строки и переменных окружения,
// приоритет отдаётся переменным окружения:
//
//	RUN_ADDRESS            | -a  — адрес и порт запуска HTTP-сервиса;
//	DATABASE_URI           | -d  — DSN подключения к PostgreSQL;
//	ACCRUAL_SYSTEM_ADDRESS | -r  — базовый адрес системы расчёта начислений.
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// Значения конфигурации по умолчанию.
const (
	// DefaultRunAddress — адрес запуска HTTP-сервиса, если он не задан явно.
	DefaultRunAddress = "localhost:8080"
	// DefaultAccrualPollInterval — период опроса системы расчёта начислений.
	DefaultAccrualPollInterval = time.Second
	// DefaultAccrualWorkers — число параллельных обработчиков заказов.
	DefaultAccrualWorkers = 5
	// DefaultAccrualBatchSize — размер пачки заказов, забираемой из БД за один проход.
	DefaultAccrualBatchSize = 20
	// DefaultTokenTTL — срок жизни выдаваемого JWT-токена.
	DefaultTokenTTL = 24 * time.Hour
	// DefaultJWTSecret — секрет подписи JWT, используемый, если не задан JWT_SECRET.
	DefaultJWTSecret = "gophermart-secret-key"
)

// ErrDatabaseURIRequired возвращается, если адрес подключения к БД не задан.
var ErrDatabaseURIRequired = errors.New("database uri is required")

// Config содержит полный набор параметров запуска сервиса.
type Config struct {
	// RunAddress — адрес и порт, на которых поднимается HTTP-сервер.
	RunAddress string
	// DatabaseURI — строка подключения к PostgreSQL.
	DatabaseURI string
	// AccrualSystemAddress — базовый адрес системы расчёта начислений.
	AccrualSystemAddress string
	// LogLevel — уровень логирования (debug, info, warn, error).
	LogLevel string
	// JWTSecret — секрет для подписи токенов аутентификации.
	JWTSecret string
	// TokenTTL — срок жизни токена аутентификации.
	TokenTTL time.Duration
	// AccrualPollInterval — интервал между опросами системы расчёта начислений.
	AccrualPollInterval time.Duration
	// AccrualWorkers — количество горутин, опрашивающих систему начислений.
	AccrualWorkers int
	// AccrualBatchSize — количество заказов, забираемых из БД за один проход.
	AccrualBatchSize int
}

// Parse собирает конфигурацию из аргументов командной строки и окружения.
// Аргумент args не должен содержать имя исполняемого файла.
func Parse(args []string) (*Config, error) {
	cfg := &Config{
		LogLevel:            "info",
		JWTSecret:           DefaultJWTSecret,
		TokenTTL:            DefaultTokenTTL,
		AccrualPollInterval: DefaultAccrualPollInterval,
		AccrualWorkers:      DefaultAccrualWorkers,
		AccrualBatchSize:    DefaultAccrualBatchSize,
	}

	fs := flag.NewFlagSet("gophermart", flag.ContinueOnError)
	fs.StringVar(&cfg.RunAddress, "a", DefaultRunAddress, "address and port to run HTTP server")
	fs.StringVar(&cfg.DatabaseURI, "d", "", "PostgreSQL connection URI")
	fs.StringVar(&cfg.AccrualSystemAddress, "r", "", "accrual system base address")
	fs.StringVar(&cfg.LogLevel, "l", cfg.LogLevel, "log level: debug, info, warn, error")
	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}

	applyEnv(cfg)

	if cfg.RunAddress == "" {
		cfg.RunAddress = DefaultRunAddress
	}
	if cfg.DatabaseURI == "" {
		return nil, ErrDatabaseURIRequired
	}
	cfg.AccrualSystemAddress = normalizeAddress(cfg.AccrualSystemAddress)

	return cfg, nil
}

// applyEnv переопределяет значения конфигурации переменными окружения.
func applyEnv(cfg *Config) {
	if v, ok := os.LookupEnv("RUN_ADDRESS"); ok && v != "" {
		cfg.RunAddress = v
	}
	if v, ok := os.LookupEnv("DATABASE_URI"); ok && v != "" {
		cfg.DatabaseURI = v
	}
	if v, ok := os.LookupEnv("ACCRUAL_SYSTEM_ADDRESS"); ok && v != "" {
		cfg.AccrualSystemAddress = v
	}
	if v, ok := os.LookupEnv("LOG_LEVEL"); ok && v != "" {
		cfg.LogLevel = v
	}
	if v, ok := os.LookupEnv("JWT_SECRET"); ok && v != "" {
		cfg.JWTSecret = v
	}
}

// normalizeAddress приводит адрес системы начислений к виду http://host:port
// без завершающего слеша.
func normalizeAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	return strings.TrimRight(addr, "/")
}
