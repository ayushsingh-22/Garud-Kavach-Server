-- Migration 009: Overtime tracking for shifts and payroll
-- Business rules:
--   * Max shift duration        = 12 hours  (hard cap)
--   * Normal hours              = min(actual_hours, 8)
--   * Overtime hours            = max(0, min(actual_hours, 12) - 8)
--   * Paid hours (effective)    = normal_hours + overtime_hours * 2
--   Example: 12 hrs worked → 8 regular + 4 OT*2 = 16 paid hours

-- ── 1. Add columns to shifts ──────────────────────────────────────────────
ALTER TABLE shifts ADD COLUMN IF NOT EXISTS overtime_hours NUMERIC(5,2) NOT NULL DEFAULT 0;
ALTER TABLE shifts ADD COLUMN IF NOT EXISTS paid_hours     NUMERIC(5,2) NOT NULL DEFAULT 0;

-- ── 2. Add columns to payroll ─────────────────────────────────────────────
ALTER TABLE payroll ADD COLUMN IF NOT EXISTS overtime_hours NUMERIC(8,2) NOT NULL DEFAULT 0;
ALTER TABLE payroll ADD COLUMN IF NOT EXISTS paid_hours     NUMERIC(8,2) NOT NULL DEFAULT 0;

-- ── 3. Fix existing shifts: cap actual_hours > 12 to exactly 12 ──────────
UPDATE shifts
SET actual_hours = 12,
    end_time     = start_time + INTERVAL '12 hours'
WHERE COALESCE(actual_hours, 0) > 12
  AND deleted_at IS NULL;

-- ── 4. Backfill overtime_hours and paid_hours for all existing shifts ─────
UPDATE shifts
SET overtime_hours = GREATEST(0, LEAST(COALESCE(actual_hours, 0), 12) - 8),
    paid_hours     = LEAST(COALESCE(actual_hours, 0), 8)
                   + GREATEST(0, LEAST(COALESCE(actual_hours, 0), 12) - 8) * 2
WHERE deleted_at IS NULL;

-- ── 5. Recompute payroll: recalculate total_pay using paid_hours * rate ───
UPDATE payroll p
SET overtime_hours = sub.total_ot,
    paid_hours     = sub.total_paid,
    total_pay      = sub.total_paid * p.rate_per_hour
FROM (
    SELECT
        s.guard_id,
        TO_CHAR(DATE_TRUNC('month', s.start_time), 'YYYY-MM') AS shift_month,
        SUM(COALESCE(s.overtime_hours, 0))                     AS total_ot,
        SUM(COALESCE(s.paid_hours, COALESCE(s.actual_hours, 0))) AS total_paid
    FROM shifts s
    WHERE s.deleted_at IS NULL
      AND s.start_time IS NOT NULL
    GROUP BY s.guard_id, DATE_TRUNC('month', s.start_time)
) sub
WHERE p.guard_id = sub.guard_id
  AND TO_CHAR(p.month, 'YYYY-MM') = sub.shift_month
  AND p.deleted_at IS NULL;
