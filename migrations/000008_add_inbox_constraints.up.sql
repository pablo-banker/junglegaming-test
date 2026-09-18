ALTER TABLE inbox_messages
    ADD CONSTRAINT pk_inbox_messages
        PRIMARY KEY (
                     consumer_name,
                     message_id
            ),

    ADD CONSTRAINT chk_inbox_messages_consumer_name
        CHECK (BTRIM(consumer_name) <> ''),

    ADD CONSTRAINT chk_inbox_messages_message_id
        CHECK (BTRIM(message_id) <> ''),

    ADD CONSTRAINT chk_inbox_messages_message_hash
        CHECK (BTRIM(message_hash) <> ''),

    ADD CONSTRAINT chk_inbox_messages_timestamps
        CHECK (
            completed_at IS NULL
                OR completed_at >= received_at
            );