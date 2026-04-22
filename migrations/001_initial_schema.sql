CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'customer' CHECK (role IN ('superadmin', 'manager', 'finance', 'hr', 'customer')),
    name TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS queries (
    id SERIAL PRIMARY KEY,
    user_id INT REFERENCES users(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    phone TEXT,
    service TEXT,
    message TEXT,
    num_guards INTEGER NOT NULL DEFAULT 1,
    duration_type TEXT,
    duration_value NUMERIC(10,2) DEFAULT 0,
    camera_required BOOLEAN DEFAULT FALSE,
    vehicle_required BOOLEAN DEFAULT FALSE,
    first_aid BOOLEAN DEFAULT FALSE,
    walkie_talkie BOOLEAN DEFAULT FALSE,
    bullet_proof BOOLEAN DEFAULT FALSE,
    fire_safety BOOLEAN DEFAULT FALSE,
    status TEXT DEFAULT 'Pending',
    cost NUMERIC(10,2) DEFAULT 0,
    submitted_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id SERIAL PRIMARY KEY,
    user_id INT REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    target TEXT NOT NULL,
    details JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_queries_status ON queries(status);
CREATE INDEX IF NOT EXISTS idx_queries_submitted_at ON queries(submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_queries_user_id ON queries(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_created ON audit_logs(user_id, created_at DESC);
