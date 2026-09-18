DROP TRIGGER IF EXISTS trg_wallet_ledger_prevent_truncate
    ON wallet_ledger_entries;

DROP TRIGGER IF EXISTS trg_wallet_ledger_prevent_update_delete
    ON wallet_ledger_entries;

DROP FUNCTION IF EXISTS prevent_wallet_ledger_mutation();


ALTER TABLE wallet_ledger_entries
    DROP CONSTRAINT IF EXISTS chk_wallet_ledger_transition,
    DROP CONSTRAINT IF EXISTS chk_wallet_ledger_balances_non_negative,
    DROP CONSTRAINT IF EXISTS chk_wallet_ledger_amount_positive,
    DROP CONSTRAINT IF EXISTS chk_wallet_ledger_currency,
    DROP CONSTRAINT IF EXISTS uq_wallet_ledger_wallet_transaction,
    DROP CONSTRAINT IF EXISTS fk_wallet_ledger_transaction,
    DROP CONSTRAINT IF EXISTS fk_wallet_ledger_wallet_currency,
    DROP CONSTRAINT IF EXISTS pk_wallet_ledger_entries;