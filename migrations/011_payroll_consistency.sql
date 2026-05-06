-- Migration 011: Comprehensive payroll consistency fix
--
-- Problem: paid_hours ≠ total_hours in 836 payroll records because:
--   1. Old migrations didn't join correctly for all guard/month combos
--   2. The paid_hours column was added with default 0 and some records were never updated
--
-- Correct invariants:
--   paid_hours     = sum(actual_hours per shift)   = total_hours   (paid per actual hour worked)
--   overtime_hours = sum(overtime_hours per shift)                  (extra hours beyond 8h/shift)
--   total_pay      = (paid_hours + overtime_hours) * rate_per_hour
--
-- Strategy:
--   Step 1: Where matching shifts exist → recompute total_hours, overtime_hours, paid_hours, total_pay from shifts
--   Step 2: Where no shifts exist (historical/seed data) → set paid_hours = total_hours, recompute total_pay

-- Step 1: Recompute all payroll records that have matching shifts
UPDATE payroll p
SET
    total_hours    = sub.total_actual,
    overtime_hours = sub.total_ot,
    paid_hours     = sub.total_actual,   -- paid_hours always = total_hours
    total_pay      = (sub.total_actual + sub.total_ot) * p.rate_per_hour
FROM (
    SELECT
        s.guard_id,
        TO_CHAR(DATE_TRUNC('month', s.start_time), 'YYYY-MM') AS shift_month,
        SUM(COALESCE(s.actual_hours, 0))                      AS total_actual,
        SUM(COALESCE(s.overtime_hours, 0))                    AS total_ot
    FROM shifts s
    WHERE s.deleted_at IS NULL
      AND s.start_time IS NOT NULL
    GROUP BY s.guard_id, DATE_TRUNC('month', s.start_time)
) sub
WHERE p.guard_id = sub.guard_id
  AND TO_CHAR(p.month, 'YYYY-MM') = sub.shift_month
  AND p.deleted_at IS NULL;

-- Step 2: For remaining records (no shift match) — ensure paid_hours = total_hours and recompute total_pay
UPDATE payroll
SET
    paid_hours = total_hours,
    total_pay  = (total_hours + COALESCE(overtime_hours, 0)) * rate_per_hour
WHERE deleted_at IS NULL
  AND ROUND(paid_hours::numeric, 2) != ROUND(total_hours::numeric, 2);
