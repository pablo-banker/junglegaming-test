REVOKE ALL
    ON wallets, wager_transactions, inbox_messages, outbox_events, wallet_ledger_entries
    FROM jungle_app;

REVOKE USAGE ON SCHEMA public FROM jungle_app;
