DROP INDEX IF EXISTS idx_wager_transactions_pending_reference_retry;


ALTER TABLE wager_transactions
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_reference_causation_id,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_reference_correlation_id,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_reference_retry_count;


ALTER TABLE wager_transactions
    DROP COLUMN IF EXISTS reference_causation_id,
    DROP COLUMN IF EXISTS reference_correlation_id,
    DROP COLUMN IF EXISTS reference_retry_count;