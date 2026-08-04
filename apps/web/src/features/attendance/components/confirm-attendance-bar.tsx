import { HvButton } from "@/components/hv";

export interface ConfirmAttendanceBarProps {
  absentCount: number;
  pending: boolean;
  /** The session's billing period is already closed — the save becomes an adjustment. */
  closedPeriod: boolean;
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
  onConfirm,
}: ConfirmAttendanceBarProps) {
  const label = closedPeriod ? "LƯU VÀ TẠO ĐIỀU CHỈNH" : "XÁC NHẬN BUỔI HỌC";
  return (
    <div className="sticky bottom-14 z-10 border-t border-line-200 bg-white p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] md:bottom-0">
      <HvButton
        type="button"
        variant={closedPeriod ? "reward" : "primary"}
        size="lg"
        block
        disabled={pending}
        onClick={onConfirm}
      >
        {pending ? "Đang lưu…" : `${label} · ${absentCount} vắng`}
      </HvButton>
    </div>
  );
}
