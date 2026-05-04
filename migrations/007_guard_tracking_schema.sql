-- Phase 9: Guard Tracking & PWA Schema
-- Run against Neon before deploying Phase 9 backend.

ALTER TABLE guards ADD COLUMN IF NOT EXISTS guard_token TEXT UNIQUE DEFAULT gen_random_uuid()::text;
ALTER TABLE guards ADD COLUMN IF NOT EXISTS clocked_in BOOLEAN DEFAULT FALSE;
ALTER TABLE guards ADD COLUMN IF NOT EXISTS clocked_in_at TIMESTAMPTZ;

-- Stores last known GPS coordinates from guard PWA
CREATE TABLE IF NOT EXISTS guard_locations (
    id          SERIAL PRIMARY KEY,
    guard_id    INT REFERENCES guards(id) ON DELETE CASCADE,
    lat         NUMERIC(10,7) NOT NULL,
    lng         NUMERIC(10,7) NOT NULL,
    recorded_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_guard_locations_guard
    ON guard_locations(guard_id, recorded_at DESC);

-- Incident reports submitted from guard PWA
CREATE TABLE IF NOT EXISTS incidents (
    id          SERIAL PRIMARY KEY,
    guard_id    INT REFERENCES guards(id) ON DELETE SET NULL,
    title       TEXT NOT NULL,
    description TEXT,
    photo_url   TEXT,
    severity    TEXT DEFAULT 'low' CHECK (severity IN ('low', 'medium', 'high', 'sos')),
    lat         NUMERIC(10,7),
    lng         NUMERIC(10,7),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_incidents_guard
    ON incidents(guard_id, created_at DESC);
