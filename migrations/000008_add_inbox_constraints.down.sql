ALTER TABLE inbox_messages
    DROP CONSTRAINT IF EXISTS chk_inbox_messages_timestamps,
    DROP CONSTRAINT IF EXISTS chk_inbox_messages_message_hash,
    DROP CONSTRAINT IF EXISTS chk_inbox_messages_message_id,
    DROP CONSTRAINT IF EXISTS chk_inbox_messages_consumer_name,
    DROP CONSTRAINT IF EXISTS pk_inbox_messages;