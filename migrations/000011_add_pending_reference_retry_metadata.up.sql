ALTER TABLE wager_transactions
    ADD COLUMN reference_retry_count    INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN reference_correlation_id TEXT,
    ADD COLUMN reference_causation_id   TEXT;


-- Retry state

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_reference_retry_count
        CHECK (reference_retry_count >= 0);


-- Reference processing metadata

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_reference_correlation_id
        CHECK (
            reference_correlation_id IS NULL
                OR BTRIM(reference_correlation_id) <> ''
            ),

    ADD CONSTRAINT chk_wager_transactions_reference_causation_id
        CHECK (
            reference_causation_id IS NULL
                OR BTRIM(reference_causation_id) <> ''
            );


-- Efficient pending reference worker lookup

CREATE INDEX idx_wager_transactions_pending_reference_retry
    ON wager_transactions (
                           next_reference_attempt_at,
                           reference_expires_at,
                           created_at
        )
    WHERE status = 'PENDING_REFERENCE'
        AND next_reference_attempt_at IS NOT NULL;