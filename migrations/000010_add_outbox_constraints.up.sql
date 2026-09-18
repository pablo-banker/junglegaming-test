-- Primary key

ALTER TABLE outbox_events
    ADD CONSTRAINT pk_outbox_events
        PRIMARY KEY (event_id);


-- Required values

ALTER TABLE outbox_events
    ADD CONSTRAINT chk_outbox_events_event_type
        CHECK (BTRIM(event_type) <> ''),

    ADD CONSTRAINT chk_outbox_events_correlation_id
        CHECK (BTRIM(correlation_id) <> ''),

    ADD CONSTRAINT chk_outbox_events_causation_id
        CHECK (
            causation_id IS NULL
                OR BTRIM(causation_id) <> ''
            ),

    ADD CONSTRAINT chk_outbox_events_version
        CHECK (version >= 1),

    ADD CONSTRAINT chk_outbox_events_attempts
        CHECK (attempts >= 0);


-- Claim/lease fields must exist together

ALTER TABLE outbox_events
    ADD CONSTRAINT chk_outbox_events_claim
        CHECK (
            (
                claimed_by IS NULL
                    AND claimed_until IS NULL
                )
                OR
            (
                claimed_by IS NOT NULL
                    AND BTRIM(claimed_by) <> ''
                    AND claimed_until IS NOT NULL
                )
            );


-- Publication cannot happen before the event occurred

ALTER TABLE outbox_events
    ADD CONSTRAINT chk_outbox_events_published_at
        CHECK (
            published_at IS NULL
                OR published_at >= occurred_at
            );


-- Efficient worker lookup

CREATE INDEX idx_outbox_events_pending
    ON outbox_events (
                      next_attempt_at,
                      claimed_until,
                      occurred_at
        )
    WHERE published_at IS NULL;


-- Event identity and payload are immutable.
-- Delivery metadata may still change.

CREATE OR REPLACE FUNCTION prevent_outbox_payload_mutation()
    RETURNS TRIGGER
    LANGUAGE plpgsql
AS
$$
BEGIN
    IF NEW.event_id IS DISTINCT FROM OLD.event_id
        OR NEW.event_type IS DISTINCT FROM OLD.event_type
        OR NEW.aggregate_id IS DISTINCT FROM OLD.aggregate_id
        OR NEW.correlation_id IS DISTINCT FROM OLD.correlation_id
        OR NEW.causation_id IS DISTINCT FROM OLD.causation_id
        OR NEW.version IS DISTINCT FROM OLD.version
        OR NEW.payload IS DISTINCT FROM OLD.payload
        OR NEW.occurred_at IS DISTINCT FROM OLD.occurred_at
    THEN
        RAISE EXCEPTION 'outbox event payload is immutable';
    END IF;

    RETURN NEW;
END;
$$;


CREATE TRIGGER trg_outbox_events_payload_immutable
    BEFORE UPDATE
    ON outbox_events
    FOR EACH ROW
EXECUTE FUNCTION prevent_outbox_payload_mutation();