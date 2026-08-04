import { Link } from "react-router";

import { HvCheckIcon } from "@/components/hv";
import { Spinner } from "@/components/shared/spinner";
import { formatSessionDate } from "@/lib/utils";

import { usePendingSessions } from "../hooks/use-dashboard";

/**
 * Manually composed to match `HvButton` variant="danger" size="sm" — HvButton
 * itself renders a `<button>`, which can't nest inside this row's `<Link>`
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
 * The prototype `home` screen's warning banner (PRD R2 AC 3): a session
 * already held but not yet attended must surface above the fold. Renders
 * nothing while loading rather than a layout-shifting skeleton, since the
 * query is fast and this sits at the very top of the dashboard.
 */
export function PendingAttendanceAlert() {
  const { data: response, isPending, isError } = usePendingSessions();

  if (isPending) {
    return (
      <div className="flex items-center justify-center rounded-[var(--radius-xl)] bg-coral-100 p-6">
        <Spinner className="size-5" />
      </div>
    );
  }

  // The teacher must be able to tell "check failed" apart from "all caught
  // up" — silently rendering nothing here would look identical to the
  // empty-state below and hide that the safety check never ran (PRD R2 AC 3).
  if (isError) {
    return (
      <p className="text-[14px] font-semibold text-coral-600">
        Không tải được danh sách buổi cần điểm danh
      </p>
    );
  }

  const { total, items } = response;

  if (total === 0) {
    return (
      <p className="flex items-center gap-2 text-[15px] font-semibold text-mint-600">
        <HvCheckIcon className="size-5" />
        Đã điểm danh đủ các buổi đã qua
      </p>
    );
  }

  return (
    <div className="rounded-[var(--radius-xl)] bg-coral-100 p-5">
      <div className="flex items-center gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-white font-display text-lg font-bold text-coral-600">
          !
        </span>
        <p className="font-display text-[16px] font-bold text-coral-600">
          {total} buổi đã qua chưa điểm danh
        </p>
      </div>
      <ul className="mt-4 flex flex-col gap-3">
        {items.map((session) => (
          <li
            key={session.session_id}
            className="flex flex-wrap items-center justify-between gap-3"
          >
            <span className="text-[14px] text-ink-700">
              {session.class_name} — {formatSessionDate(session.session_date)}
            </span>
            <Link
              to={`/sessions/${session.session_id}/attendance`}
              className={dangerLinkButtonClassName}
            >
              Điểm danh ngay
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
