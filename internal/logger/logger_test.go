package logger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/d2cTool/gomart/internal/logger"
)

func TestNew(t *testing.T) {
	log, err := logger.New("debug")
	require.NoError(t, err)
	require.NotNil(t, log)
	assert.True(t, log.Core().Enabled(-1))
}

func TestNewInvalidLevel(t *testing.T) {
	_, err := logger.New("verbose")
	assert.Error(t, err)
}
