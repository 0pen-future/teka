import { useState } from "react";

import { HvModal } from "@/components/hv";
import { cn, formatSessionDate } from "@/lib/utils";

import { useSessionsList } from "../hooks/use-sessions";
import {
  bySessionOrder,
  mondayFirstWeekday,
  monthBounds,
  monthDays,
  shiftMonth,
} from "../lib/session-dates";
import { sessionStatusInfo } from "../lib/session-status-info";
import type { Session } from "../schemas/attendance-schemas";

const WEEKDAY_HEADERS = ["T2", "T3", "T4", "T5", "T6", "T7", "CN"];

export interface MonthCalendarModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  classId: string;
  /** Month (`YYYY-MM`) the grid opens on — normally the anchor's month. */
  initialMonth: string;
  today: string;
  /** Called when a day with sessions is tapped; the page navigates. */
  onPickSession: (session: Session) => void;
}

function monthTitle(month: string): string {
  const [year, monthNumber] = month.split("-");
  return `Tháng ${Number(monthNumber)}/${year}`;
}

/**
 * Month-grid shortcut for jumping far from the trio's window. Hand-built on
 * design-system tokens (7-column CSS grid, one query per viewed month through
 * the existing sessions hook) instead of a calendar dependency — the scope is
 * only "show dots, pick a day".
 */
export function MonthCalendarModal({
  open,
  onOpenChange,
  classId,
  initialMonth,
  today,
  onPickSession,
}: MonthCalendarModalProps) {
  const [month, setMonth] = useState(initialMonth);
  const { data: sessions } = useSessionsList(classId, monthBounds(month));

  const sessionsByDay = new Map<string, Session[]>();
  for (const session of [...(sessions ?? [])].sort(bySessionOrder)) {
    const bucket = sessionsByDay.get(session.session_date) ?? [];
    bucket.push(session);
    sessionsByDay.set(session.session_date, bucket);
  }

  const days = monthDays(month);
  const leadingBlanks = mondayFirstWeekday(days[0]!);

  return (
    <HvModal open={open} onOpenChange={onOpenChange} title="Lịch tháng">
      <div className="mb-3 flex items-center justify-between">
        <button
          type="button"
          aria-label="Tháng trước"
          onClick={() => setMonth((current) => shiftMonth(current, -1))}
          className="flex size-11 items-center justify-center rounded-full text-[18px] font-extrabold text-ink-500 transition-colors hover:bg-cream-100 focus-visible:outline-none focus-visible:ring-4"
        >
          ‹
        </button>
        <span className="font-display text-[15px] font-extrabold text-ink-900">
          {monthTitle(month)}
        </span>
        <button
          type="button"
          aria-label="Tháng sau"
          onClick={() => setMonth((current) => shiftMonth(current, 1))}
          className="flex size-11 items-center justify-center rounded-full text-[18px] font-extrabold text-ink-500 transition-colors hover:bg-cream-100 focus-visible:outline-none focus-visible:ring-4"
        >
          ›
        </button>
      </div>

      <div className="grid grid-cols-7 gap-1 text-center">
        {WEEKDAY_HEADERS.map((weekday) => (
          <span
            key={weekday}
            className="py-1 text-[11px] font-extrabold uppercase tracking-[0.4px] text-ink-400"
          >
            {weekday}
          </span>
        ))}
        {Array.from({ length: leadingBlanks }, (_, index) => (
          <span key={`blank-${index}`} />
        ))}
        {days.map((day) => {
          const daySessions = sessionsByDay.get(day);
          const dayNumber = Number(day.slice(8, 10));
          if (!daySessions) {
            return (
              <span
                key={day}
                className={cn(
                  "flex h-11 items-center justify-center text-[13px] font-semibold text-ink-400",
                  day === today && "rounded-full ring-2 ring-mint-300",
                )}
              >
                {dayNumber}
              </span>
            );
          }
          // One dot per distinct status on the day; the first session (by
          // chronological order) is the tap target.
          const dotClasses = Array.from(
            new Set(daySessions.map((session) => sessionStatusInfo(session, today).dotClass)),
          );
          return (
            <button
              key={day}
              type="button"
              aria-label={formatSessionDate(day)}
              onClick={() => onPickSession(daySessions[0]!)}
              className={cn(
                "flex h-11 flex-col items-center justify-center gap-[3px] rounded-[10px] text-[13px] font-extrabold text-ink-900 transition-colors hover:bg-cream-100 focus-visible:outline-none focus-visible:ring-4",
                day === today && "ring-2 ring-mint-300",
              )}
            >
              {dayNumber}
              <span className="flex gap-[3px]">
                {dotClasses.map((dotClass) => (
                  <span key={dotClass} className={cn("size-[6px] rounded-full", dotClass)} />
                ))}
              </span>
            </button>
          );
        })}
      </div>
    </HvModal>
  );
}
