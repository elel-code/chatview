CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    pub_key     TEXT PRIMARY KEY,
    role        SMALLINT NOT NULL DEFAULT 0,
    status      SMALLINT NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    token       TEXT PRIMARY KEY,
    pub_key     TEXT NOT NULL REFERENCES users(pub_key) ON DELETE CASCADE,
    client_id   TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    is_online   BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_sessions_pub_key ON sessions(pub_key);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at) WHERE is_online;

CREATE TABLE IF NOT EXISTS challenges (
    pub_key     TEXT PRIMARY KEY,
    challenge   BYTEA NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_challenges_expires ON challenges(expires_at);

CREATE TABLE IF NOT EXISTS friendships (
    user_pub_key    TEXT NOT NULL REFERENCES users(pub_key) ON DELETE CASCADE,
    friend_pub_key  TEXT NOT NULL REFERENCES users(pub_key) ON DELETE CASCADE,
    alias           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_pub_key, friend_pub_key)
);

CREATE INDEX IF NOT EXISTS idx_friendships_friend ON friendships(friend_pub_key);

CREATE TABLE IF NOT EXISTS conversations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    participant_a   TEXT NOT NULL REFERENCES users(pub_key) ON DELETE CASCADE,
    participant_b   TEXT NOT NULL REFERENCES users(pub_key) ON DELETE CASCADE,
    next_seq        BIGINT NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (participant_a, participant_b),
    CHECK (participant_a < participant_b)
);

CREATE TABLE IF NOT EXISTS messages (
    id               UUID PRIMARY KEY DEFAULT uuidv7(),
    conversation_id  UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_pub_key   TEXT NOT NULL REFERENCES users(pub_key) ON DELETE CASCADE,
    text             TEXT NOT NULL,
    timestamp        TIMESTAMPTZ NOT NULL DEFAULT now(),
    server_seq       BIGINT NOT NULL,
    UNIQUE (conversation_id, server_seq)
);

CREATE INDEX IF NOT EXISTS idx_messages_conv_seq ON messages(conversation_id, server_seq DESC);
CREATE INDEX IF NOT EXISTS idx_messages_conv_ts ON messages(conversation_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender_pub_key);
