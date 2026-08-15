package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/d2cTool/gomart/internal/models"
)

func TestNewPoints(t *testing.T) {
	assert.Equal(t, models.Points(50050), models.NewPoints(500.5))
	assert.Equal(t, models.Points(4200), models.NewPoints(42))
	assert.Equal(t, models.Points(1), models.NewPoints(0.011))
	assert.Equal(t, models.Points(0), models.NewPoints(0))
}

func TestPointsFloat64AndString(t *testing.T) {
	assert.InDelta(t, 500.5, models.Points(50050).Float64(), 1e-9)
	assert.Equal(t, "500.5", models.Points(50050).String())
	assert.Equal(t, "42", models.Points(4200).String())
	assert.Equal(t, "0", models.Points(0).String())
}

func TestPointsMarshalJSON(t *testing.T) {
	data, err := json.Marshal(models.Balance{Current: models.NewPoints(500.5), Withdrawn: models.NewPoints(42)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"current":500.5,"withdrawn":42}`, string(data))
}

func TestPointsUnmarshalJSON(t *testing.T) {
	var payload struct {
		Sum models.Points `json:"sum"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"sum":751.25}`), &payload))
	assert.Equal(t, models.Points(75125), payload.Sum)

	require.Error(t, json.Unmarshal([]byte(`{"sum":"not-a-number"}`), &payload))
}

func TestOrderMarshalJSON(t *testing.T) {
	accrual := models.NewPoints(500)
	uploaded := time.Date(2020, 12, 10, 15, 15, 45, 0, time.FixedZone("MSK", 3*60*60))

	data, err := json.Marshal(models.Order{
		Number:     "9278923470",
		Status:     models.StatusProcessed,
		Accrual:    &accrual,
		UploadedAt: uploaded,
		UserID:     7,
	})
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"number":"9278923470","status":"PROCESSED","accrual":500,"uploaded_at":"2020-12-10T15:15:45+03:00"}`,
		string(data))
}

func TestOrderWithoutAccrualOmitsField(t *testing.T) {
	data, err := json.Marshal(models.Order{
		Number:     "12345678903",
		Status:     models.StatusProcessing,
		UploadedAt: time.Date(2020, 12, 10, 15, 12, 1, 0, time.UTC),
	})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "accrual")
}

func TestOrderStatusIsFinal(t *testing.T) {
	assert.True(t, models.StatusProcessed.IsFinal())
	assert.True(t, models.StatusInvalid.IsFinal())
	assert.False(t, models.StatusNew.IsFinal())
	assert.False(t, models.StatusProcessing.IsFinal())
}

func TestAccrualStatusOrderStatus(t *testing.T) {
	tests := map[models.AccrualStatus]models.OrderStatus{
		models.AccrualRegistered: models.StatusProcessing,
		models.AccrualProcessing: models.StatusProcessing,
		models.AccrualProcessed:  models.StatusProcessed,
		models.AccrualInvalid:    models.StatusInvalid,
		models.AccrualStatus(""): models.StatusProcessing,
	}
	for accrualStatus, want := range tests {
		assert.Equal(t, want, accrualStatus.OrderStatus(), "status %q", accrualStatus)
	}
}

func TestTooManyRequestsError(t *testing.T) {
	err := &models.TooManyRequestsError{RetryAfterSeconds: 60, Message: "No more than 10 requests per minute allowed"}
	assert.Equal(t, "No more than 10 requests per minute allowed", err.Error())
	assert.Equal(t, "too many requests to the accrual system", (&models.TooManyRequestsError{}).Error())
}
