-- Reverses 000001_baseline_schema: drops every object the baseline created,
-- in reverse dependency order (views first, then children before parents).
-- The pgcrypto extension is intentionally kept: it may pre-exist and is shared.

DROP VIEW IF EXISTS v_unbilled_attendance;
DROP VIEW IF EXISTS v_contact_balance;

DROP TABLE IF EXISTS notifications CASCADE;
DROP TABLE IF EXISTS statements CASCADE;
DROP TABLE IF EXISTS payment_allocations CASCADE;
DROP TABLE IF EXISTS payments CASCADE;
DROP TABLE IF EXISTS invoice_adjustments CASCADE;
DROP TABLE IF EXISTS invoice_lines CASCADE;
DROP TABLE IF EXISTS invoices CASCADE;
DROP TABLE IF EXISTS billing_periods CASCADE;
DROP TABLE IF EXISTS attendance_records CASCADE;
DROP TABLE IF EXISTS class_sessions CASCADE;
DROP TABLE IF EXISTS enrollments CASCADE;
DROP TABLE IF EXISTS class_schedules CASCADE;
DROP TABLE IF EXISTS classes CASCADE;
DROP TABLE IF EXISTS students CASCADE;
DROP TABLE IF EXISTS contacts CASCADE;
DROP TABLE IF EXISTS teachers CASCADE;
DROP TABLE IF EXISTS user_accounts CASCADE;
