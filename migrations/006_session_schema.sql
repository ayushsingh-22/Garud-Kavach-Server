-- 006_session_schema.sql
-- Refresh token sessions for the stateful revocation strategy (Phase 7.3).

CREATE TABLE IF NOT EXISTS user_sessions (
    id                SERIAL PRIMARY KEY,
    user_id           INT REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL,
    expires_at        TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    revoked_at        TIMESTAMPTZ DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id      ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at   ON user_sessions(expires_at);
