ALTER TABLE wallets
    ADD CONSTRAINT pk_wallets
        PRIMARY KEY (id),

    ADD CONSTRAINT uq_wallets_player_currency
        UNIQUE (player_id, currency),

    ADD CONSTRAINT uq_wallets_id_currency
        UNIQUE (id, currency),

    ADD CONSTRAINT chk_wallets_currency
        CHECK (currency ~ '^[A-Z]{3}$'),

    ADD CONSTRAINT chk_wallets_balance_non_negative
        CHECK (balance >= 0),

    ADD CONSTRAINT chk_wallets_version_positive
        CHECK (version >= 1),

    ADD CONSTRAINT chk_wallets_timestamps
        CHECK (updated_at >= created_at);