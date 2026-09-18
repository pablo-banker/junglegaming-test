CREATE TYPE wager_transaction_type AS ENUM (
    'OPENING',
    'BET',
    'WIN',
    'LOSS',
    'REFUND',
    'ROLLBACK'
    );

CREATE TYPE wager_transaction_status AS ENUM (
    'PENDING',
    'PENDING_REFERENCE',
    'PROCESSED',
    'REJECTED',
    'FAILED'
    );

CREATE TABLE wager_transactions
(
    id                                UUID                     NOT NULL,

    provider_id                       TEXT,
    external_transaction_id           TEXT,
    idempotency_key                   TEXT,
    payload_hash                      TEXT,

    wallet_id                         UUID                     NOT NULL,
    player_id                         UUID                     NOT NULL,

    round_id                          TEXT,
    game_id                           TEXT,

    type                              wager_transaction_type   NOT NULL,

    amount                            NUMERIC(20, 2)           NOT NULL,
    currency                          VARCHAR(3)               NOT NULL,

    reference_external_transaction_id TEXT,
    reference_transaction_id          UUID,
    reference_transaction_type        wager_transaction_type,

    status                            wager_transaction_status NOT NULL,

    failure_code                      TEXT,
    failure_message                   TEXT,

    balance_before                    NUMERIC(20, 2),
    balance_after                     NUMERIC(20, 2),

    reference_attempts                INTEGER                  NOT NULL DEFAULT 0,
    next_reference_attempt_at         TIMESTAMPTZ,
    reference_expires_at              TIMESTAMPTZ,

    created_at                        TIMESTAMPTZ              NOT NULL DEFAULT NOW(),
    updated_at                        TIMESTAMPTZ              NOT NULL DEFAULT NOW(),
    completed_at                      TIMESTAMPTZ
);