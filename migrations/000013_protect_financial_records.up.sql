-- Terminal wager transactions are immutable and business fields never change.
-- The application enforces the same rules in the domain; the database is the last line of defense.

CREATE OR REPLACE FUNCTION prevent_wager_transaction_mutation()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS
$$
BEGIN
    IF OLD.status IN ('PROCESSED', 'REJECTED', 'FAILED') THEN
        RAISE EXCEPTION 'wager transaction % is terminal and cannot change', OLD.id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.provider_id IS DISTINCT FROM OLD.provider_id
        OR NEW.external_transaction_id IS DISTINCT FROM OLD.external_transaction_id
        OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
        OR NEW.payload_hash IS DISTINCT FROM OLD.payload_hash
        OR NEW.wallet_id IS DISTINCT FROM OLD.wallet_id
        OR NEW.player_id IS DISTINCT FROM OLD.player_id
        OR NEW.round_id IS DISTINCT FROM OLD.round_id
        OR NEW.game_id IS DISTINCT FROM OLD.game_id
        OR NEW.type IS DISTINCT FROM OLD.type
        OR NEW.amount IS DISTINCT FROM OLD.amount
        OR NEW.currency IS DISTINCT FROM OLD.currency
        OR NEW.reference_external_transaction_id IS DISTINCT FROM OLD.reference_external_transaction_id
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'wager transaction % business fields are immutable', OLD.id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    RETURN NEW;
END;
$$;


CREATE TRIGGER trg_wager_transactions_immutable
    BEFORE UPDATE
    ON wager_transactions
    FOR EACH ROW
EXECUTE FUNCTION prevent_wager_transaction_mutation();


-- Wallet identity never changes and the version moves by exactly one
-- only when the balance changes.

CREATE OR REPLACE FUNCTION enforce_wallet_update_rules()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS
$$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.player_id IS DISTINCT FROM OLD.player_id
        OR NEW.currency IS DISTINCT FROM OLD.currency
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'wallet % identity is immutable', OLD.id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    IF NEW.balance IS DISTINCT FROM OLD.balance THEN
        IF NEW.version <> OLD.version + 1 THEN
            RAISE EXCEPTION 'wallet % version must increase by one on balance change', OLD.id
                USING ERRCODE = 'integrity_constraint_violation';
        END IF;
    ELSIF NEW.version <> OLD.version THEN
        RAISE EXCEPTION 'wallet % version can only change with its balance', OLD.id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;

    RETURN NEW;
END;
$$;


CREATE TRIGGER trg_wallets_update_rules
    BEFORE UPDATE
    ON wallets
    FOR EACH ROW
EXECUTE FUNCTION enforce_wallet_update_rules();
