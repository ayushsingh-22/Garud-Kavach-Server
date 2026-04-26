-- Migration for Phase 4: Finance & HR Portals

CREATE TABLE IF NOT EXISTS invoices (
    id SERIAL PRIMARY KEY,
    query_id INT REFERENCES queries(id),
    amount NUMERIC(10,2) NOT NULL,
    status TEXT DEFAULT 'pending',
    issued_at TIMESTAMPTZ DEFAULT NOW(),
    paid_at TIMESTAMPTZ,
    payment_ref TEXT,
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS expenses (
    id SERIAL PRIMARY KEY,
    category TEXT,
    description TEXT,
    amount NUMERIC(10,2) NOT NULL,
    expense_date DATE NOT NULL,
    added_by INT REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS shifts (
    id SERIAL PRIMARY KEY,
    guard_id INT REFERENCES guards(id),
    query_id INT REFERENCES queries(id),
    start_time TIMESTAMPTZ,
    end_time TIMESTAMPTZ,
    actual_hours NUMERIC(5,2),
    status TEXT DEFAULT 'scheduled',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS payroll (
    id SERIAL PRIMARY KEY,
    guard_id INT REFERENCES guards(id),
    month DATE NOT NULL,
    total_hours NUMERIC(8,2),
    rate_per_hour NUMERIC(8,2),
    total_pay NUMERIC(10,2),
    status TEXT DEFAULT 'pending',
    paid_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS leave_requests (
    id SERIAL PRIMARY KEY,
    guard_id INT REFERENCES guards(id),
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    reason TEXT,
    status TEXT DEFAULT 'pending',
    reviewed_by INT REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_invoices_query_id ON invoices(query_id);
CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status);
CREATE INDEX IF NOT EXISTS idx_shifts_guard_id ON shifts(guard_id);
CREATE INDEX IF NOT EXISTS idx_payroll_guard_month ON payroll(guard_id, month);