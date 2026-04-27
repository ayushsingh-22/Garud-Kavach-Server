-- Phase 6: Notifications schema
CREATE TABLE IF NOT EXISTS notifications (
    id          SERIAL PRIMARY KEY,
    user_id     INT REFERENCES users(id) ON DELETE CASCADE,
    message     TEXT NOT NULL,
    type        TEXT DEFAULT 'info' CHECK (type IN ('info', 'warning', 'success', 'error')),
    read        BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
    ON notifications (user_id, read) WHERE read = FALSE;
