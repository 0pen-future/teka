import { HvButton } from "@/components/hv";

export interface ConfirmAttendanceBarProps {
  absentCount: number;
  lateCount: number;
  pending: boolean;
  /** The session's billing period is already closed — the save becomes an adjustment. */
  closedPeriod: boolean;
  /**
   * Confirmed server-side with no local edits — the button reports state
   * instead of offering a save. Derived once in `AttendancePage` so the label
   * and the confirm handler can never disagree.
   */
  settled: boolean;
  /** Whether the viewer may confirm attendance — hoc_vu class staff and handed-off teachers read only. */
  canWrite: boolean;
  /**
   * False while the class (and with it the viewer's staff roles) is still
   * loading — the bar waits in a neutral disabled state instead of flashing
   * the denial label at a teacher who may write.
   */
  accessResolved: boolean;
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
  lateCount,
  pending,
  closedPeriod,
  settled,
  canWrite,
  accessResolved,
  onConfirm,
}: ConfirmAttendanceBarProps) {
  // "XÁC NHẬN · n VẮNG · n MUỘN" — only the non-zero exception counts are
  // appended, so the everyone-on-time default stays the plain one-word tap.
  const exceptionSuffix =
    (absentCount > 0 ? ` · ${absentCount} VẮNG` : "") +
    (lateCount > 0 ? ` · ${lateCount} MUỘN` : "");
  const label = !accessResolved
    ? "Đang tải…"
    : !canWrite
      ? "CHỈ GIÁO VIÊN, TRỢ GIẢNG LỚP HOẶC CHỦ TRUNG TÂM MỚI XÁC NHẬN ĐƯỢC"
      : settled
        ? "ĐÃ XÁC NHẬN ✓"
        : `${closedPeriod ? "LƯU VÀ TẠO ĐIỀU CHỈNH" : "XÁC NHẬN"}${exceptionSuffix}`;
  return (
    <div className="sticky bottom-14 z-10 rounded-b-[28px] border-t border-line-100 bg-white px-4 py-[14px] pb-[max(14px,env(safe-area-inset-bottom))] md:bottom-0">
      <HvButton
        type="button"
        variant={closedPeriod && !settled ? "reward" : "primary"}
        size="lg"
        block
        disabled={pending || !canWrite}
        onClick={onConfirm}
      >
        {pending ? "Đang lưu…" : label}
      </HvButton>
    </div>
  );
}
