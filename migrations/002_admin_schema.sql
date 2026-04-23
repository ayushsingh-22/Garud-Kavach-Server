-- Migration for Phase 3: Admin, Manager & Guard Management

CREATE TABLE IF NOT EXISTS guards (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    phone TEXT,
    email TEXT,
    address TEXT,
    license_no TEXT,
    license_expiry DATE,
    status TEXT DEFAULT 'active',
    hourly_rate NUMERIC(8,2) DEFAULT 0,
    photo_url TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS guard_query_assignments (
    id SERIAL PRIMARY KEY,
    guard_id INT REFERENCES guards(id),
    query_id INT REFERENCES queries(id),
    assigned_at TIMESTAMPTZ DEFAULT NOW(),
    unassigned_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_guards_status ON guards(status);
CREATE INDEX IF NOT EXISTS idx_guards_license_expiry ON guards(license_expiry);
CREATE INDEX IF NOT EXISTS idx_assignments_query ON guard_query_assignments(query_id);
