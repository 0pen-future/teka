import { Link } from "react-router";

import { cn, formatSessionDate } from "@/lib/utils";

import { formatAttendanceSummary, sessionStatusInfo } from "../lib/session-status-info";
import type { Session } from "../schemas/attendance-schemas";

export interface SessionTrioPickerProps {
  /** Session right before the anchor in the class's sequence, if any. */
  prev: Session | null;
  /** The session the picker is centered on (route-selected or default). */
  anchor: Session | null;
  /** Session right after the anchor, if any. */
  next: Session | null;
  /** Today's ISO date — decides the HÔM NAY caption and status coloring. */
  today: string;
  /**
   * Called with the target session on every arrow/card activation. The page
   * owns navigation and window recentering; the picker is purely visual.
   */
  onNavigate: (session: Session) => void;
}

function TrioCard({
  caption,
  session,
  isAnchor,
  today,
  onNavigate,
}: {
  caption: string;
  session: Session | null;
  isAnchor: boolean;
  today: string;
  onNavigate: (session: Session) => void;
}) {
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-1">
      <span className="px-1 text-[11px] font-extrabold uppercase tracking-[0.4px] text-ink-400">
        {caption}
      </span>
      {session ? (
        <Link
          to={`/sessions/${session.id}/attendance`}
          onClick={() => onNavigate(session)}
          className={cn(
            "flex min-h-[64px] flex-col justify-center gap-[2px] rounded-[14px] border-2 px-3 py-2 transition-colors",
            isAnchor ? "border-mint-300 bg-mint-50" : "border-line-100 bg-white hover:bg-cream-100",
          )}
        >
          <span className="truncate text-[13.5px] font-extrabold text-ink-900">
            {formatSessionDate(session.session_date)}
            {session.start_time ? (
              <span className="font-semibold text-ink-400"> · {session.start_time}</span>
            ) : null}
          </span>
          <span
            className={cn("text-[12.5px] font-bold", sessionStatusInfo(session, today).textClass)}
          >
            {sessionStatusInfo(session, today).text}
          </span>
          {session.attendance_summary ? (
            <span className="text-[11.5px] font-semibold text-ink-500">
              {formatAttendanceSummary(session.attendance_summary)}
            </span>
          ) : null}
        </Link>
      ) : (
        <div className="flex min-h-[64px] items-center justify-center rounded-[14px] border-2 border-dashed border-line-200 text-[12.5px] font-semibold text-ink-300">
          Chưa có buổi
        </div>
      )}
    </div>
  );
}

function ArrowButton({
  label,
  glyph,
  target,
  onNavigate,
  className,
}: {
  label: string;
  glyph: string;
  target: Session | null;
  onNavigate: (session: Session) => void;
  className?: string;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      disabled={!target}
      onClick={() => {
        if (target) {
          onNavigate(target);
        }
      }}
      className={cn(
        "flex size-11 shrink-0 items-center justify-center self-center rounded-full font-display text-[18px] font-extrabold transition-colors focus-visible:outline-none focus-visible:ring-4",
        target
          ? "bg-white text-ink-500 shadow-soft-sm hover:bg-cream-100"
          : "bg-cream-100 text-ink-300",
        className,
      )}
    >
      {glyph}
    </button>
  );
}

/**
 * The TRƯỚC · HÔM NAY/ĐANG XEM · KẾ TIẾP session picker. Horizontal above the
 * table on mobile, vertical in the left column at `lg+` — same DOM, flipped
 * flex direction. The anchor lives in the route (`/sessions/:id/attendance`),
 * so every activation here is plain navigation, never extra state.
 */
export function SessionTrioPicker({
  prev,
  anchor,
  next,
  today,
  onNavigate,
}: SessionTrioPickerProps) {
  const anchorCaption = anchor?.session_date === today ? "HÔM NAY" : "ĐANG XEM";
  return (
    <div className="flex flex-row items-stretch gap-2 lg:flex-col">
      {/* `contents` keeps both arrows direct flex items of the mobile row
          (next pushed to the end via `max-lg:order-last`); at lg+ the wrapper
          becomes a real row so the arrows share one line above the cards.
          Accepted trade-off: on mobile the next arrow's tab/reading order
          (right after prev) no longer matches its visual position at the end
          of the row — harmless here since both buttons carry distinct
          self-describing aria-labels. */}
      <div className="contents lg:flex lg:flex-row lg:justify-between">
        <ArrowButton label="Buổi trước" glyph="‹" target={prev} onNavigate={onNavigate} />
        <ArrowButton
          label="Buổi kế tiếp"
          glyph="›"
          target={next}
          onNavigate={onNavigate}
          className="max-lg:order-last"
        />
      </div>
      <TrioCard
        caption="TRƯỚC"
        session={prev}
        isAnchor={false}
        today={today}
        onNavigate={onNavigate}
      />
      <TrioCard
        caption={anchorCaption}
        session={anchor}
        isAnchor
        today={today}
        onNavigate={onNavigate}
      />
      <TrioCard
        caption="KẾ TIẾP"
        session={next}
        isAnchor={false}
        today={today}
        onNavigate={onNavigate}
      />
    </div>
  );
}
