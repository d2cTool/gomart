package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/d2cTool/gomart/internal/config"
)

func TestParseFlags(t *testing.T) {
	cfg, err := config.Parse([]string{
		"-a", "127.0.0.1:9090",
		"-d", "postgres://user:pass@localhost:5432/praktikum",
		"-r", "localhost:8081",
	})
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:9090", cfg.RunAddress)
	assert.Equal(t, "postgres://user:pass@localhost:5432/praktikum", cfg.DatabaseURI)
	assert.Equal(t, "http://localhost:8081", cfg.AccrualSystemAddress)
	assert.Equal(t, config.DefaultTokenTTL, cfg.TokenTTL)
	assert.Positive(t, cfg.AccrualWorkers)
}

func TestParseEnvOverridesFlags(t *testing.T) {
	t.Setenv("RUN_ADDRESS", "0.0.0.0:8888")
	t.Setenv("DATABASE_URI", "postgres://env/db")
	t.Setenv("ACCRUAL_SYSTEM_ADDRESS", "http://accrual:8080/")
	t.Setenv("JWT_SECRET", "env-secret")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := config.Parse([]string{"-a", "127.0.0.1:9090", "-d", "postgres://flag/db", "-r", "flag:1234"})
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0:8888", cfg.RunAddress)
	assert.Equal(t, "postgres://env/db", cfg.DatabaseURI)
	assert.Equal(t, "http://accrual:8080", cfg.AccrualSystemAddress)
	assert.Equal(t, "env-secret", cfg.JWTSecret)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestParseRequiresDatabaseURI(t *testing.T) {
	_, err := config.Parse([]string{"-a", "localhost:8080"})
	assert.ErrorIs(t, err, config.ErrDatabaseURIRequired)
}

func TestParseDefaults(t *testing.T) {
	cfg, err := config.Parse([]string{"-d", "postgres://localhost/db"})
	require.NoError(t, err)

	assert.Equal(t, config.DefaultRunAddress, cfg.RunAddress)
	assert.Empty(t, cfg.AccrualSystemAddress)
	assert.Equal(t, config.DefaultAccrualPollInterval, cfg.AccrualPollInterval)
	assert.Equal(t, 24*time.Hour, cfg.TokenTTL)
}

func TestParseUnknownFlag(t *testing.T) {
	_, err := config.Parse([]string{"-unknown"})
	assert.Error(t, err)
}

func TestParseKeepsSchemeOfAccrualAddress(t *testing.T) {
	cfg, err := config.Parse([]string{"-d", "postgres://localhost/db", "-r", "https://accrual.example.com/"})
	require.NoError(t, err)
	assert.Equal(t, "https://accrual.example.com", cfg.AccrualSystemAddress)
}
