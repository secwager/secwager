CREATE TABLE btc_deposits (
    txid           VARCHAR(64)  NOT NULL,
    vout           INTEGER      NOT NULL,
    user_id        VARCHAR(64)  NOT NULL,
    satoshis       BIGINT       NOT NULL CHECK (satoshis > 0),
    seen_at_height INTEGER      NOT NULL,
    escrowed       BOOLEAN      NOT NULL DEFAULT FALSE,
    confirmed      BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (txid, vout)
);

CREATE INDEX idx_btc_deposits_active ON btc_deposits(seen_at_height)
    WHERE confirmed = FALSE;

CREATE INDEX idx_btc_deposits_user ON btc_deposits(user_id, created_at);
