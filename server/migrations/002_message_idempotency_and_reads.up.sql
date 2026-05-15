ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS client_message_id TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_sender_client_message
    ON messages(sender_pub_key, client_message_id)
    WHERE client_message_id <> '';

CREATE TABLE IF NOT EXISTS conversation_reads (
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_pub_key    TEXT NOT NULL REFERENCES users(pub_key) ON DELETE CASCADE,
    last_read_seq   BIGINT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, user_pub_key)
);

CREATE INDEX IF NOT EXISTS idx_conversation_reads_user
    ON conversation_reads(user_pub_key);
