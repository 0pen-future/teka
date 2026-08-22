import { Link } from "react-router";

import { cn, formatSessionDate } from "@/lib/utils";

import { usePendingSessions } from "../hooks/use-dashboard";

/**
 * Manually composed to match `HvButton` variant="danger" size="md" — HvButton
 * itself renders a `<button>`, which can't wrap this banner's navigation
 * without producing invalid nested-interactive-element markup, so the link is
 * styled directly from the same token utilities instead.
 */
const dangerLinkButtonClassName =
  "inline-flex min-h-[44px] shrink-0 items-center justify-center rounded-[var(--radius-md)] " +
  "bg-coral-400 px-[18px] text-[length:var(--text-sm)] font-display font-bold text-white " +
  "shadow-press-coral transition-[transform,box-shadow,filter] duration-[var(--dur-fast)] " +
  "ease-[var(--ease-out)] hover:brightness-[1.04] active:translate-y-[var(--press-depth)] " +
  "active:shadow-none focus-visible:outline-none focus-visible:ring-4";

/**
 * The prototype `home` screen's warning banner (PRD R2 AC 3): sessions
 * already held but not yet attended must surface above the fold, as one
 * merged banner whose single action opens the first listed pending session
 * (the server returns newest first). Renders nothing while loading or when
 * caught up — the prototype shows the banner only when there is something
 * to fix.
 */
export function PendingAttendanceAlert({ className }: { className?: string }) {
  const { data: response, isPending, isError } = usePendingSessions();

  if (isPending) {
    return null;
  }

  // The teacher must be able to tell "check failed" apart from "all caught
  // up" — silently rendering nothing here would look identical to the
  // caught-up state and hide that the safety check never ran (PRD R2 AC 3).
  if (isError) {
    return (
      <p className={cn("text-[14px] font-semibold text-coral-600", className)}>
        Không tải được danh sách buổi cần điểm danh
      </p>
    );
  }

  const { total, items } = response;
  const first = items[0];

  if (total === 0 || !first) {
    return null;
  }

  // The headline counts everything (`total` is unbounded); the detail line
  // stays one readable sentence — a teacher weeks behind must still see the
  // stats below the fold, not a wall of session labels.
  const listed = items.slice(0, 3);
  const extraCount = total - listed.length;
  const sessionList =
    listed
      .map((session) => `${session.class_name} — ${formatSessionDate(session.session_date)}`)
      .join(" · ") + (extraCount > 0 ? ` … và ${extraCount} buổi khác` : "");

  return (
    <div
      className={cn(
        "flex flex-wrap items-center gap-[14px] rounded-[var(--radius-lg)] bg-coral-100 px-5 py-4",
        className,
      )}
    >
      <span
        aria-hidden
        className="flex size-10 shrink-0 items-center justify-center rounded-full bg-coral-400 font-display text-[20px] font-black text-white"
      >
        !
      </span>
      <div className="min-w-[220px] flex-1">
        <p className="text-[15.5px] font-extrabold text-coral-600">
          Có {total} buổi đã dạy nhưng chưa điểm danh
        </p>
        <p className="text-[14px] text-ink-700">
          {sessionList}. Chưa điểm danh là chưa tính được tiền.
        </p>
      </div>
      <Link to={`/sessions/${first.session_id}/attendance`} className={dangerLinkButtonClassName}>
        Điểm danh ngay
      </Link>
    </div>
  );
}
