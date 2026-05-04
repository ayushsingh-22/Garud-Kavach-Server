-- Migration 008: Geofence support for queries + auto-assign indexes
-- Applied via Neon MCP on session start (columns were added in same session as 007).
-- This file documents the schema changes for version control.

-- Geofence columns on queries table
ALTER TABLE queries
  ADD COLUMN IF NOT EXISTS geofence_lat      NUMERIC(10,7),
  ADD COLUMN IF NOT EXISTS geofence_lng      NUMERIC(10,7),
  ADD COLUMN IF NOT EXISTS geofence_radius_m INTEGER DEFAULT 500,
  ADD COLUMN IF NOT EXISTS service_date      TIMESTAMPTZ;

-- Index for shift scheduling queries by service date
CREATE INDEX IF NOT EXISTS idx_queries_service_date
  ON queries(service_date)
  WHERE deleted_at IS NULL;

-- Index for fast "latest location per guard" lookups (used by auto-assign scoring)
CREATE INDEX IF NOT EXISTS idx_guard_locations_guard_latest
  ON guard_locations(guard_id, recorded_at DESC);
