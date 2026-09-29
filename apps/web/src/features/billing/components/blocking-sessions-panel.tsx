import { Link } from "react-router";

import { hvButtonVariants } from "@/components/hv";
import { cn, formatSessionDate } from "@/lib/utils";

import type { BlockingSession } from "../schemas/billing-schemas";

export interface BlockingSessionsPanelProps {
  sessions: BlockingSession[];
}

interface ClassGroup {
  classId: string;
  className: string;
  sessions: BlockingSession[];
}

/** Groups sessions by class, keeping the server's order within and across groups. */
function groupByClass(sessions: BlockingSession[]): ClassGroup[] {
  const groups = new Map<string, ClassGroup>();
  for (const session of sessions) {
    const group = groups.get(session.class_id) ?? {
      classId: session.class_id,
      className: session.class_name,
      sessions: [],
    };
    group.sessions.push(session);
    groups.set(session.class_id, group);
  }
  return [...groups.values()];
}

/** The most overdue session: the one a single "start here" action should open. */
function mostOverdue(sessions: BlockingSession[]): BlockingSession | undefined {
  return sessions.reduce<BlockingSession | undefined>(
    (oldest, session) => (!oldest || session.days_overdue > oldest.days_overdue ? session : oldest),
    undefined,
  );
}

/**
 * Chốt sổ (`close`) prototype's blocked panel: `--coral-100` bg,
 * `--radius-xl`, title `--coral-600`. Mirrors the server-side gate
 * (`close.go`'s `blockingSessions()`) — every past session in the period
 * without confirmed attendance must be resolved before the close button
 * enables, both here and (authoritatively) on the server.
 *
 * One primary action opens the most overdue session; the full list stays
 * reachable as plain links grouped per class in collapsed `<details>`, so a
 * long backlog never becomes a wall of buttons.
 */
export function BlockingSessionsPanel({ sessions }: BlockingSessionsPanelProps) {
  const first = mostOverdue(sessions);
  if (!first) {
    return null;
  }

  return (
    <div className="rounded-[var(--radius-xl)] bg-coral-100 p-5" role="alert">
      <p className="font-display text-[16px] font-bold text-coral-600">Chưa thể chốt sổ</p>
      <p className="mt-1 text-[13px] text-ink-600">
        Còn {sessions.length} buổi học đã qua chưa được điểm danh. Chốt sổ chỉ khả dụng sau khi mọi
        buổi học đã qua trong kỳ được điểm danh.
      </p>
      <Link
        to={`/sessions/${first.session_id}/attendance`}
        className={cn(hvButtonVariants({ variant: "danger", size: "md" }), "mt-4")}
      >
        Điểm danh buổi còn thiếu ({sessions.length})
      </Link>
      <div className="mt-4 flex flex-col gap-2">
        {groupByClass(sessions).map((group) => (
          <details key={group.classId} className="rounded-[var(--radius-sm)] bg-white/60">
            <summary className="flex min-h-11 cursor-pointer items-center px-3 text-[14px] font-bold text-ink-700">
              {group.className} — {group.sessions.length} buổi
            </summary>
            <ul className="flex flex-col px-3 pb-2">
              {group.sessions.map((session) => (
                <li key={session.session_id}>
                  <Link
                    to={`/sessions/${session.session_id}/attendance`}
                    aria-label={`Điểm danh ${group.className}, ${formatSessionDate(session.session_date)}`}
                    className="flex min-h-11 items-center font-bold text-coral-600 underline underline-offset-4"
                  >
                    {formatSessionDate(session.session_date)}
                  </Link>
                </li>
              ))}
            </ul>
          </details>
        ))}
      </div>
    </div>
  );
}
