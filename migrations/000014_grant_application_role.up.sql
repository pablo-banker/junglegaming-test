-- Least privilege role used by the application at runtime.
-- The migration owner keeps DDL rights. The application can insert and read the ledger
-- but never update, delete or truncate it, and cannot delete any financial record.
-- Its login password is set outside migrations (docker/postgres/01-app-role.sh).

DO
$$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'jungle_app') THEN
        CREATE ROLE jungle_app NOLOGIN;
    END IF;
END
$$;


GRANT USAGE ON SCHEMA public TO jungle_app;

GRANT SELECT, INSERT, UPDATE
    ON wallets, wager_transactions, inbox_messages, outbox_events
    TO jungle_app;

GRANT SELECT, INSERT
    ON wallet_ledger_entries
    TO jungle_app;
