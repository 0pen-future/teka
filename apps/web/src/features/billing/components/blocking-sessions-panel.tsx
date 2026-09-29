import { Link } from "react-router";

import { hvButtonVariants } from "@/components/hv";
import { cn, formatSessionDate } from "@/lib/utils";

import type { BlockingSession } from "../schemas/billing-schemas";

/**
 * `HvButton` renders a `<button>`, which can't nest inside this row's
 * `<Link>`, so the link takes the button's classes from `hvButtonVariants`.
 */
const dangerLinkButtonClassName = cn(
  hvButtonVariants({ variant: "danger", size: "sm" }),
  "shrink-0",
);

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
