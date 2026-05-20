CREATE TABLE accounts (
    user_id       VARCHAR(64)  PRIMARY KEY,
    gross_balance BIGINT       NOT NULL DEFAULT 0 CHECK (gross_balance >= 0),
    escrowed      BIGINT       NOT NULL DEFAULT 0 CHECK (escrowed >= 0),
    version       BIGINT       NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT coverage CHECK (gross_balance >= escrowed)
);

CREATE TABLE escrow_entries (
    order_id   BIGINT       PRIMARY KEY,
    user_id    VARCHAR(64)  NOT NULL REFERENCES accounts(user_id),
    amount     BIGINT       NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_escrow_user ON escrow_entries(user_id);

CREATE TABLE idempotency_keys (
    key        TEXT        PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
