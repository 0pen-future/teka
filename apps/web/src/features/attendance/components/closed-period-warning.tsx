/**
 * PRD R4 AC 2: editing attendance whose date falls in an already-closed
 * billing period does not reopen that period — the difference posts as an
 * adjustment (`invoice_adjustments.source_session_id`,
 * `docs/schema_design.sql:344`) in the next one. This banner makes that
 * consequence legible before the teacher commits.
 */
export function ClosedPeriodWarning() {
  return (
    <div
      className="rounded-[var(--radius-lg)] bg-sun-100 p-3 text-[13px] font-semibold text-sun-600"
      role="alert"
    >
      Kỳ thu học phí của buổi này đã chốt sổ. Thay đổi điểm danh sẽ tạo một khoản điều chỉnh ở kỳ kế
      tiếp, không sửa hoá đơn đã chốt.
    </div>
  );
}
