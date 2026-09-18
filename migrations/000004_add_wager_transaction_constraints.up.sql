-- Primary key

ALTER TABLE wager_transactions
    ADD CONSTRAINT pk_wager_transactions
        PRIMARY KEY (id);


-- Relationships

ALTER TABLE wager_transactions
    ADD CONSTRAINT fk_wager_transactions_wallet_currency
        FOREIGN KEY (wallet_id, currency)
            REFERENCES wallets (id, currency);


-- Composite keys used by relationships

ALTER TABLE wager_transactions
    ADD CONSTRAINT uq_wager_transactions_id_type
        UNIQUE (id, type),

    ADD CONSTRAINT uq_wager_transactions_id_wallet_currency
        UNIQUE (id, wallet_id, currency);


-- Internal reference

ALTER TABLE wager_transactions
    ADD CONSTRAINT fk_wager_transactions_reference
        FOREIGN KEY (
                     reference_transaction_id,
                     reference_transaction_type
            )
            REFERENCES wager_transactions (id, type);


-- Currency

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_currency
        CHECK (currency ~ '^[A-Z]{3}$');


-- Amount by transaction type

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_amount
        CHECK (
            (
                type = 'LOSS'
                    AND amount = 0
                )
                OR
            (
                type <> 'LOSS'
                    AND amount > 0
                )
            );


-- Persisted balances can never be negative

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_balances
        CHECK (
            (balance_before IS NULL OR balance_before >= 0)
                AND
            (balance_after IS NULL OR balance_after >= 0)
            );


-- OPENING is internal.
-- Every other type originates from an external provider.

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_origin
        CHECK (
            (
                type = 'OPENING'
                    AND provider_id IS NULL
                    AND external_transaction_id IS NULL
                    AND idempotency_key IS NULL
                    AND payload_hash IS NULL
                    AND round_id IS NULL
                    AND game_id IS NULL
                    AND reference_external_transaction_id IS NULL
                    AND reference_transaction_id IS NULL
                    AND reference_transaction_type IS NULL
                    AND status = 'PROCESSED'
                )
                OR
            (
                type <> 'OPENING'
                    AND provider_id IS NOT NULL
                    AND BTRIM(provider_id) <> ''
                    AND external_transaction_id IS NOT NULL
                    AND BTRIM(external_transaction_id) <> ''
                    AND idempotency_key IS NOT NULL
                    AND BTRIM(idempotency_key) <> ''
                    AND payload_hash IS NOT NULL
                    AND BTRIM(payload_hash) <> ''
                    AND round_id IS NOT NULL
                    AND BTRIM(round_id) <> ''
                    AND game_id IS NOT NULL
                    AND BTRIM(game_id) <> ''
                )
            );


-- External reference cannot be blank when present

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_reference_external_not_blank
        CHECK (
            reference_external_transaction_id IS NULL
                OR BTRIM(reference_external_transaction_id) <> ''
            );


-- Internal reference ID and type must always exist together

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_reference_pair
        CHECK (
            (
                reference_transaction_id IS NULL
                    AND reference_transaction_type IS NULL
                )
                OR
            (
                reference_transaction_id IS NOT NULL
                    AND reference_transaction_type IS NOT NULL
                    AND reference_external_transaction_id IS NOT NULL
                )
            );


-- Reference rules by transaction type

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_reference_by_type
        CHECK (
            (
                type IN ('OPENING', 'BET', 'LOSS')
                    AND reference_external_transaction_id IS NULL
                    AND reference_transaction_id IS NULL
                    AND reference_transaction_type IS NULL
                )
                OR
            (
                type = 'WIN'
                    AND (
                    reference_transaction_type IS NULL
                        OR reference_transaction_type = 'BET'
                    )
                )
                OR
            (
                type = 'REFUND'
                    AND reference_external_transaction_id IS NOT NULL
                    AND (
                    reference_transaction_type IS NULL
                        OR reference_transaction_type = 'BET'
                    )
                )
                OR
            (
                type = 'ROLLBACK'
                    AND reference_external_transaction_id IS NOT NULL
                    AND (
                    reference_transaction_type IS NULL
                        OR reference_transaction_type IN (
                                                          'BET',
                                                          'WIN',
                                                          'REFUND'
                        )
                    )
                )
            );


-- Fields allowed for each status

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_status_fields
        CHECK (
            (
                status IN ('PENDING', 'PENDING_REFERENCE')
                    AND completed_at IS NULL
                    AND balance_before IS NULL
                    AND balance_after IS NULL
                    AND failure_code IS NULL
                    AND failure_message IS NULL
                )
                OR
            (
                status = 'PROCESSED'
                    AND completed_at IS NOT NULL
                    AND balance_before IS NOT NULL
                    AND balance_after IS NOT NULL
                    AND failure_code IS NULL
                    AND failure_message IS NULL
                )
                OR
            (
                status IN ('REJECTED', 'FAILED')
                    AND completed_at IS NOT NULL
                    AND balance_before IS NULL
                    AND balance_after IS NULL
                    AND failure_code IS NOT NULL
                    AND BTRIM(failure_code) <> ''
                )
            );


-- PENDING_REFERENCE means the external reference exists,
-- but its internal transaction has not been resolved yet.

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_pending_reference
        CHECK (
            status <> 'PENDING_REFERENCE'
                OR (
                reference_external_transaction_id IS NOT NULL
                    AND reference_transaction_id IS NULL
                    AND reference_transaction_type IS NULL
                    AND next_reference_attempt_at IS NOT NULL
                    AND reference_expires_at IS NOT NULL
                )
            );


-- Processed operations that informed a reference
-- must have resolved it.

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_processed_reference
        CHECK (
            status <> 'PROCESSED'
                OR
            (
                type IN ('OPENING', 'BET', 'LOSS')
                )
                OR
            (
                type = 'WIN'
                    AND (
                    reference_external_transaction_id IS NULL
                        OR reference_transaction_id IS NOT NULL
                    )
                )
                OR
            (
                type IN ('REFUND', 'ROLLBACK')
                    AND reference_transaction_id IS NOT NULL
                )
            );


-- Financial result of a successfully processed transaction

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_processed_result
        CHECK (
            status <> 'PROCESSED'
                OR (
                CASE
                    WHEN type = 'OPENING' THEN
                        balance_before = 0
                            AND balance_after = amount

                    WHEN type = 'BET' THEN
                        balance_after = balance_before - amount

                    WHEN type = 'WIN' THEN
                        balance_after = balance_before + amount

                    WHEN type = 'LOSS' THEN
                        balance_after = balance_before

                    WHEN type = 'REFUND' THEN
                        balance_after = balance_before + amount

                    WHEN type = 'ROLLBACK'
                        AND reference_transaction_type = 'BET' THEN
                        balance_after = balance_before + amount

                    WHEN type = 'ROLLBACK'
                        AND reference_transaction_type IN ('WIN', 'REFUND') THEN
                        balance_after = balance_before - amount

                    ELSE FALSE
                    END
                )
            );


-- Reference retry metadata

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_reference_attempts
        CHECK (reference_attempts >= 0),

    ADD CONSTRAINT chk_wager_transactions_reference_expiration
        CHECK (
            reference_expires_at IS NULL
                OR reference_expires_at >= created_at
            ),

    ADD CONSTRAINT chk_wager_transactions_reference_retry_window
        CHECK (
            next_reference_attempt_at IS NULL
                OR reference_expires_at IS NULL
                OR next_reference_attempt_at <= reference_expires_at
            );


-- Timestamps

ALTER TABLE wager_transactions
    ADD CONSTRAINT chk_wager_transactions_timestamps
        CHECK (
            updated_at >= created_at
                AND (
                completed_at IS NULL
                    OR (
                    completed_at >= created_at
                        AND completed_at <= updated_at
                    )
                )
            );


-- Persistent idempotency

CREATE UNIQUE INDEX uq_wager_transactions_provider_external
    ON wager_transactions (
                           provider_id,
                           external_transaction_id
        )
    WHERE type <> 'OPENING';


CREATE UNIQUE INDEX uq_wager_transactions_provider_idempotency
    ON wager_transactions (
                           provider_id,
                           idempotency_key
        )
    WHERE type <> 'OPENING';


-- One OPENING per wallet

CREATE UNIQUE INDEX uq_wager_transactions_opening_wallet
    ON wager_transactions (wallet_id)
    WHERE type = 'OPENING';


-- One direct successful financial reversal per reference

CREATE UNIQUE INDEX uq_wager_transactions_successful_reversal
    ON wager_transactions (reference_transaction_id)
    WHERE status = 'PROCESSED'
        AND type IN ('REFUND', 'ROLLBACK');


-- Pending reference worker lookup

CREATE INDEX idx_wager_transactions_pending_reference
    ON wager_transactions (
                           next_reference_attempt_at,
                           created_at
        )
    WHERE status = 'PENDING_REFERENCE';