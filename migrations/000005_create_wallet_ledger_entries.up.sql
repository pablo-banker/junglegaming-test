CREATE TYPE wallet_ledger_direction AS ENUM (
    'DEBIT',
    'CREDIT'
    );

CREATE TABLE wallet_ledger_entries
(
    id             UUID                    NOT NULL,

    wallet_id      UUID                    NOT NULL,
    transaction_id UUID                    NOT NULL,

    direction      wallet_ledger_direction NOT NULL,

    amount         NUMERIC(20, 2)          NOT NULL,
    currency       VARCHAR(3)              NOT NULL,

    balance_before NUMERIC(20, 2)          NOT NULL,
    balance_after  NUMERIC(20, 2)          NOT NULL,

    created_at     TIMESTAMPTZ             NOT NULL DEFAULT NOW()
);