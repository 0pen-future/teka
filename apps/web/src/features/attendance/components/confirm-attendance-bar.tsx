import { HvButton } from "@/components/hv";

export interface ConfirmAttendanceBarProps {
  absentCount: number;
  pending: boolean;
  /** The session's billing period is already closed — the save becomes an adjustment. */
  closedPeriod: boolean;
  /**
   * Confirmed server-side with no local edits — the button reports state
   * instead of offering a save. Derived once in `AttendancePage` so the label
   * and the confirm handler can never disagree.
   */
  settled: boolean;
  onConfirm: () => void;
}

/**
 * Sticks to the viewport bottom as the page scrolls (its ancestors keep no
 * inner overflow, so it resolves against the document scroll, not the end of a
 * 30-row list) — confirming stays reachable at any scroll position, which the
 * one-touch interaction budget depends on. `bottom-14` clears the mobile tab
 * bar; `md:bottom-0` sits flush once that bar is gone.
 */
export function ConfirmAttendanceBar({
  absentCount,
  pending,
  closedPeriod,
  settled,
  onConfirm,
}: ConfirmAttendanceBarProps) {
  const label = settled
    ? "ĐÃ XÁC NHẬN ✓"
    : `${closedPeriod ? "LƯU VÀ TẠO ĐIỀU CHỈNH" : "XÁC NHẬN BUỔI HỌC"} · ${absentCount} vắng`;
  return (
    <div className="sticky bottom-14 z-10 rounded-b-[28px] border-t border-line-100 bg-white px-4 py-[14px] pb-[max(14px,env(safe-area-inset-bottom))] md:bottom-0">
      <HvButton
        type="button"
        variant={closedPeriod && !settled ? "reward" : "primary"}
        size="lg"
        block
        disabled={pending}
        onClick={onConfirm}
      >
        {pending ? "Đang lưu…" : label}
      </HvButton>
    </div>
  );
}
