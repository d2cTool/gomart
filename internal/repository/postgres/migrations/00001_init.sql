-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    login         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS balances (
    user_id   BIGINT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    current   BIGINT NOT NULL DEFAULT 0 CHECK (current >= 0),
    withdrawn BIGINT NOT NULL DEFAULT 0 CHECK (withdrawn >= 0)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS orders (
    number      TEXT PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status      TEXT        NOT NULL DEFAULT 'NEW',
    accrual     BIGINT      NOT NULL DEFAULT 0 CHECK (accrual >= 0),
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS orders_user_uploaded_at_idx
    ON orders (user_id, uploaded_at DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS orders_pending_idx
    ON orders (updated_at)
    WHERE status IN ('NEW', 'PROCESSING');
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS withdrawals (
    id           BIGSERIAL PRIMARY KEY,
    order_number TEXT        NOT NULL UNIQUE,
    user_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    sum          BIGINT      NOT NULL CHECK (sum > 0),
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS withdrawals_user_processed_at_idx
    ON withdrawals (user_id, processed_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS withdrawals;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS orders;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS balances;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
