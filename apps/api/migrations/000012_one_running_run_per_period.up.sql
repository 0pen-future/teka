-- With the delegated report-sender permission (can_send_reports), two
-- different people (the period's teacher and a secretary, or two secretaries)
-- can now legally reach the send path for the SAME billing period. The
-- per-teacher index (000006) cannot see that collision, so one period's
-- parents could be DM'd twice by two concurrent passes. Backstop the
-- per-period invariant in the database itself.
--
-- Tenancy note: the center-composite FKs from 000007 already admit a run or
-- notification whose teacher_id is a different center member than the
-- period's teacher, so delegated attribution needs no constraint change.
--
-- Defensive sweep first: the app never produces two running runs for one
-- period today, but if a crashed deploy ever left duplicates behind, keep
-- only the newest running run per period and mark the rest interrupted (the
-- boot reconciler's terminal state) so index creation succeeds.
UPDATE notification_runs nr
SET status = 'interrupted', finished_at = now()
WHERE nr.status = 'running'
  AND nr.id <> (
      SELECT nr2.id
      FROM notification_runs nr2
      WHERE nr2.billing_period_id = nr.billing_period_id
        AND nr2.status = 'running'
      ORDER BY nr2.created_at DESC, nr2.id DESC
      LIMIT 1
  );

CREATE UNIQUE INDEX uq_notification_runs_one_active_period
    ON notification_runs(billing_period_id)
    WHERE status = 'running';
