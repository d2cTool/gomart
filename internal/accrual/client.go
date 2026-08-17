// Package accrual реализует HTTP-клиент внешней системы расчёта баллов
// лояльности.
package accrual

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/d2cTool/gomart/internal/models"
)

// DefaultTimeout — таймаут запроса к системе расчёта начислений по умолчанию.
const DefaultTimeout = 5 * time.Second

// defaultRetryAfter — пауза, применяемая при ответе 429 без заголовка Retry-After.
const defaultRetryAfter = 60

// Client обращается к системе расчёта баллов лояльности.
type Client struct {
	baseURL string
	client  *http.Client
}

// New создаёт клиента системы расчёта начислений с таймаутом по умолчанию.
func New(baseURL string) *Client {
	return NewWithClient(baseURL, &http.Client{Timeout: DefaultTimeout})
}

// NewWithClient создаёт клиента системы расчёта начислений с указанным
// HTTP-клиентом. Используется в тестах и при нестандартных настройках
// транспорта.
func NewWithClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), client: httpClient}
}

// OrderInfo запрашивает информацию о расчёте начисления по номеру заказа.
//
// Возвращает models.ErrOrderNotRegistered, если заказ неизвестен системе
// расчёта, и *models.TooManyRequestsError при превышении лимита запросов.
func (c *Client) OrderInfo(ctx context.Context, number string) (models.AccrualInfo, error) {
	url := c.baseURL + "/api/orders/" + number
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.AccrualInfo{}, fmt.Errorf("create accrual request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return models.AccrualInfo{}, fmt.Errorf("call accrual system: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := readBody(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		if readErr != nil {
			return models.AccrualInfo{}, readErr
		}
		var info models.AccrualInfo
		if err := json.Unmarshal(body, &info); err != nil {
			return models.AccrualInfo{}, fmt.Errorf("decode accrual response: %w", err)
		}
		if info.Order == "" {
			info.Order = number
		}
		return info, nil
	case http.StatusNoContent:
		return models.AccrualInfo{}, models.ErrOrderNotRegistered
	case http.StatusTooManyRequests:
		return models.AccrualInfo{}, &models.TooManyRequestsError{
			RetryAfterSeconds: retryAfter(resp.Header.Get("Retry-After")),
			Message:           strings.TrimSpace(string(body)),
		}
	default:
		return models.AccrualInfo{}, fmt.Errorf("accrual system returned status %d", resp.StatusCode)
	}
}

func readBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		_, _ = io.Copy(io.Discard, body)
		return nil, fmt.Errorf("read accrual response: %w", err)
	}
	return data, nil
}

// retryAfter разбирает заголовок Retry-After, подставляя значение
// по умолчанию, если заголовок отсутствует или некорректен.
func retryAfter(header string) int {
	seconds, err := strconv.Atoi(strings.TrimSpace(header))
	if err != nil || seconds <= 0 {
		return defaultRetryAfter
	}
	return seconds
}
