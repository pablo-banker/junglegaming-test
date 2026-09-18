CREATE TABLE wallets
(
    id         UUID           NOT NULL,
    player_id  UUID           NOT NULL,
    currency   VARCHAR(3)     NOT NULL,
    balance    NUMERIC(20, 2) NOT NULL,
    version    BIGINT         NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);