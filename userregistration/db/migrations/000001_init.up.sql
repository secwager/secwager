CREATE TABLE users (
    user_id           VARCHAR(64)  PRIMARY KEY,  -- Cognito sub (UUID)
    username          VARCHAR(64)  NOT NULL UNIQUE,
    btc_pubkey        BYTEA        NOT NULL,       -- compressed 33-byte secp256k1 key
    encrypted_privkey BYTEA        NOT NULL,       -- KMS ciphertext blob
    kms_key_id        VARCHAR(256) NOT NULL,       -- KMS key ARN used (for rotation tracking)
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP
);
