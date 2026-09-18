DROP TRIGGER IF EXISTS trg_outbox_events_payload_immutable
    ON outbox_events;

DROP FUNCTION IF EXISTS prevent_outbox_payload_mutation();

DROP INDEX IF EXISTS idx_outbox_events_pending;


ALTER TABLE outbox_events
    DROP CONSTRAINT IF EXISTS chk_outbox_events_published_at,
    DROP CONSTRAINT IF EXISTS chk_outbox_events_claim,
    DROP CONSTRAINT IF EXISTS chk_outbox_events_attempts,
    DROP CONSTRAINT IF EXISTS chk_outbox_events_version,
    DROP CONSTRAINT IF EXISTS chk_outbox_events_causation_id,
    DROP CONSTRAINT IF EXISTS chk_outbox_events_correlation_id,
    DROP CONSTRAINT IF EXISTS chk_outbox_events_event_type,
    DROP CONSTRAINT IF EXISTS pk_outbox_events;