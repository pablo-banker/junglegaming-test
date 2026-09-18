-- Primary key

ALTER TABLE wallet_ledger_entries
    ADD CONSTRAINT pk_wallet_ledger_entries
        PRIMARY KEY (id);


-- Wallet and currency must agree

ALTER TABLE wallet_ledger_entries
    ADD CONSTRAINT fk_wallet_ledger_wallet_currency
        FOREIGN KEY (wallet_id, currency)
            REFERENCES wallets (id, currency);


-- Ledger transaction must belong to this same wallet and currency

ALTER TABLE wallet_ledger_entries
    ADD CONSTRAINT fk_wallet_ledger_transaction
        FOREIGN KEY (
                     transaction_id,
                     wallet_id,
                     currency
            )
            REFERENCES wager_transactions (
                                           id,
                                           wallet_id,
                                           currency
                );


-- Only one movement for a transaction in a wallet

ALTER TABLE wallet_ledger_entries
    ADD CONSTRAINT uq_wallet_ledger_wallet_transaction
        UNIQUE (wallet_id, transaction_id);


-- Financial invariants

ALTER TABLE wallet_ledger_entries
    ADD CONSTRAINT chk_wallet_ledger_currency
        CHECK (currency ~ '^[A-Z]{3}$'),

    ADD CONSTRAINT chk_wallet_ledger_amount_positive
        CHECK (amount > 0),

    ADD CONSTRAINT chk_wallet_ledger_balances_non_negative
        CHECK (
            balance_before >= 0
                AND balance_after >= 0
            ),

    ADD CONSTRAINT chk_wallet_ledger_transition
        CHECK (
            (
                direction = 'DEBIT'
                    AND balance_after = balance_before - amount
                )
                OR
            (
                direction = 'CREDIT'
                    AND balance_after = balance_before + amount
                )
            );


-- Ledger is append-only

CREATE OR REPLACE FUNCTION prevent_wallet_ledger_mutation()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS
$$
BEGIN
    RAISE EXCEPTION 'wallet ledger is append-only';
END;
$$;


CREATE TRIGGER trg_wallet_ledger_prevent_update_delete
    BEFORE UPDATE OR DELETE
    ON wallet_ledger_entries
    FOR EACH ROW
EXECUTE FUNCTION prevent_wallet_ledger_mutation();


CREATE TRIGGER trg_wallet_ledger_prevent_truncate
    BEFORE TRUNCATE
    ON wallet_ledger_entries
    FOR EACH STATEMENT
EXECUTE FUNCTION prevent_wallet_ledger_mutation();