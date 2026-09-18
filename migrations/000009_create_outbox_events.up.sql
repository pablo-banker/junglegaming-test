CREATE TABLE outbox_events
(
    event_id        UUID        NOT NULL,

    event_type      TEXT        NOT NULL,
    aggregate_id    UUID        NOT NULL,

    correlation_id  TEXT        NOT NULL,
    causation_id    TEXT,

    version         BIGINT      NOT NULL,

    payload         JSONB       NOT NULL,

    occurred_at     TIMESTAMPTZ NOT NULL,

    attempts        INTEGER     NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    claimed_by      TEXT,
    claimed_until   TIMESTAMPTZ,

    published_at    TIMESTAMPTZ,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);