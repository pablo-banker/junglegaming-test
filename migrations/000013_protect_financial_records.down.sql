DROP TRIGGER IF EXISTS trg_wallets_update_rules
    ON wallets;

DROP FUNCTION IF EXISTS enforce_wallet_update_rules();


DROP TRIGGER IF EXISTS trg_wager_transactions_immutable
    ON wager_transactions;

DROP FUNCTION IF EXISTS prevent_wager_transaction_mutation();
