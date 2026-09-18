ALTER TABLE wallets
    DROP CONSTRAINT IF EXISTS chk_wallets_timestamps,
    DROP CONSTRAINT IF EXISTS chk_wallets_version_positive,
    DROP CONSTRAINT IF EXISTS chk_wallets_balance_non_negative,
    DROP CONSTRAINT IF EXISTS chk_wallets_currency,
    DROP CONSTRAINT IF EXISTS uq_wallets_id_currency,
    DROP CONSTRAINT IF EXISTS uq_wallets_player_currency,
    DROP CONSTRAINT IF EXISTS pk_wallets;