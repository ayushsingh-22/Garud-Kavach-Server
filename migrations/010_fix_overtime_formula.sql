-- Migration 010: Fix overtime formula
-- Old formula: paid_hours = 8 + overtime*2  (could reach 16 for 12h shift) ← WRONG
-- New formula: paid_hours = actual_hours (capped at 12)                      ← CORRECT
--              overtime_hours = max(0, actual_hours - 8)
--              total_pay = (paid_hours + overtime_hours) * rate_per_hour

-- Step 1: Recompute shifts.paid_hours and shifts.overtime_hours
UPDATE shifts
SET
    overtime_hours = GREATEST(0, LEAST(COALESCE(actual_hours, 0), 12) - 8),
    paid_hours     = LEAST(COALESCE(actual_hours, 0), 12)
WHERE deleted_at IS NULL;

-- Step 2: Recompute payroll totals (paid_hours, overtime_hours, total_pay) from fixed shifts
UPDATE payroll p
SET
    overtime_hours = sub.total_ot,
    paid_hours     = sub.total_paid,
    total_pay      = (sub.total_paid + sub.total_ot) * p.rate_per_hour
FROM (
    SELECT
        s.guard_id,
        TO_CHAR(DATE_TRUNC('month', s.start_time), 'YYYY-MM') AS shift_month,
        SUM(COALESCE(s.overtime_hours, 0))                    AS total_ot,
        SUM(COALESCE(s.paid_hours, COALESCE(s.actual_hours, 0))) AS total_paid
    FROM shifts s
    WHERE s.deleted_at IS NULL
      AND s.start_time IS NOT NULL
    GROUP BY s.guard_id, DATE_TRUNC('month', s.start_time)
) sub
WHERE p.guard_id = sub.guard_id
  AND TO_CHAR(p.month, 'YYYY-MM') = sub.shift_month
  AND p.deleted_at IS NULL;
