-- reference_retry_count (000011) replaced the never used reference_attempts column,
-- and idx_wager_transactions_pending_reference_retry (000011) supersedes the older index.

ALTER TABLE wager_transactions
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_reference_attempts;

ALTER TABLE wager_transactions
    DROP COLUMN IF EXISTS reference_attempts;

DROP INDEX IF EXISTS idx_wager_transactions_pending_reference;
