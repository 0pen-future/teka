import { Link } from "react-router";

import { formatSessionDate } from "@/lib/utils";

import type { BlockingSession } from "../schemas/billing-schemas";

/**
 * Manually composed to match `HvButton` variant="danger" size="sm" — `HvButton`
 * itself renders a `<button>`, which can't nest inside this row's `<Link>`
 * without producing invalid nested-interactive-element markup (same
 * constraint as `PendingAttendanceAlert`'s `dangerLinkButtonClassName`).
 */
const dangerLinkButtonClassName =
  "inline-flex min-h-[44px] shrink-0 items-center justify-center rounded-[var(--radius-md)] " +
  "bg-coral-400 px-[18px] text-[length:var(--text-sm)] font-display font-bold text-white " +
  "shadow-press-coral transition-[transform,box-shadow,filter] duration-[var(--dur-fast)] " +
  "ease-[var(--ease-out)] hover:brightness-[1.04] active:translate-y-[var(--press-depth)] " +
  "active:shadow-none focus-visible:outline-none focus-visible:ring-4";

export interface BlockingSessionsPanelProps {
  sessions: BlockingSession[];
}

/**
 * Chốt sổ (`close`) prototype's blocked panel: `--coral-100` bg,
 * `--radius-xl`, title `--coral-600`. Mirrors the server-side gate
 * (`close.go`'s `blockingSessions()`) — every past session in the period
 * without confirmed attendance must be resolved before the close button
 * enables, both here and (authoritatively) on the server.
 */
export function BlockingSessionsPanel({ sessions }: BlockingSessionsPanelProps) {
  if (sessions.length === 0) {
    return null;
  }

  return (
    <div className="rounded-[var(--radius-xl)] bg-coral-100 p-5" role="alert">
      <p className="font-display text-[16px] font-bold text-coral-600">Chưa thể chốt sổ</p>
      <p className="mt-1 text-[13px] text-ink-600">
        Còn {sessions.length} buổi học đã qua chưa được điểm danh. Chốt sổ chỉ khả dụng sau khi mọi
        buổi học đã qua trong kỳ được điểm danh.
      </p>
      <ul className="mt-4 flex flex-col gap-3">
        {sessions.map((session) => (
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
              Điểm danh
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
