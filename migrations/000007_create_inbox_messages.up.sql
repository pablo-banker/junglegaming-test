CREATE TABLE inbox_messages
(
    consumer_name TEXT        NOT NULL,
    message_id    TEXT        NOT NULL,
    message_hash  TEXT        NOT NULL,

    received_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at  TIMESTAMPTZ
);