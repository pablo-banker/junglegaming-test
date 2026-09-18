DROP INDEX IF EXISTS idx_wager_transactions_pending_reference;
DROP INDEX IF EXISTS uq_wager_transactions_successful_reversal;
DROP INDEX IF EXISTS uq_wager_transactions_opening_wallet;
DROP INDEX IF EXISTS uq_wager_transactions_provider_idempotency;
DROP INDEX IF EXISTS uq_wager_transactions_provider_external;


ALTER TABLE wager_transactions
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_timestamps,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_reference_retry_window,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_reference_expiration,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_reference_attempts,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_processed_result,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_processed_reference,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_pending_reference,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_status_fields,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_reference_by_type,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_reference_pair,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_reference_external_not_blank,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_origin,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_balances,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_amount,
    DROP CONSTRAINT IF EXISTS chk_wager_transactions_currency,
    DROP CONSTRAINT IF EXISTS fk_wager_transactions_reference,
    DROP CONSTRAINT IF EXISTS uq_wager_transactions_id_wallet_currency,
    DROP CONSTRAINT IF EXISTS uq_wager_transactions_id_type,
    DROP CONSTRAINT IF EXISTS fk_wager_transactions_wallet_currency,
    DROP CONSTRAINT IF EXISTS pk_wager_transactions;