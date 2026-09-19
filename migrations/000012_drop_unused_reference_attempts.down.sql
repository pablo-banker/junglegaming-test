CREATE INDEX IF NOT EXISTS idx_wager_transactions_pending_reference
    ON wager_transactions (
                           next_reference_attempt_at,
                           created_at
        )
    WHERE status = 'PENDING_REFERENCE';

ALTER TABLE wager_transactions
    ADD COLUMN IF NOT EXISTS reference_attempts INTEGER NOT NULL DEFAULT 0;

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_reference_attempts
        CHECK (reference_attempts >= 0);
