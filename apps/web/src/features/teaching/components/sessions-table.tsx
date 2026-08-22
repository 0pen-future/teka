import { formatSessionDate } from "@/lib/utils";
import { cn } from "@/lib/utils";

import type { SessionDerived } from "../lib/classbook-stats";
import { lessonPlanKey, type TeachingState } from "../lib/teaching-store";
import { vnd } from "../lib/vnd";
import { PlanStatusPill } from "./plan-status-pill";

interface SessionsTableProps {
  classId: string;
  derived: SessionDerived[];
  teaching: TeachingState;
  selectedSessionId: string | null;
  onSelect: (sessionId: string) => void;
}

const rowGridClassName = "grid grid-cols-[78px_92px_62px_64px_104px_1fr] items-center gap-2";

/**
 * The classbook's per-session table. Column semantics per prototype: every
 * non-value renders "—" — a cancelled session everywhere, a planned session
 * in the confirmed-only columns, a held session whose roster is still
 * loading in the roster-derived ones.
 */
export function SessionsTable({
  classId,
  derived,
  teaching,
  selectedSessionId,
  onSelect,
}: SessionsTableProps) {
  return (
    <div className="overflow-hidden rounded-[24px] bg-white shadow-soft-md">
      <div className="overflow-x-auto">
        <div className="min-w-[560px]">
          <div
            className={cn(
              rowGridClassName,
              "border-b-[1.5px] border-line-200 px-4 py-3 text-[11.5px] font-extrabold tracking-[0.3px] text-ink-400",
            )}
          >
            <div>BUỔI</div>
            <div>GIÁO ÁN</div>
            <div>SĨ SỐ</div>
            <div>ĐIỂM TB</div>
            <div>DOANH THU</div>
            <div>NHẬN XÉT CHUNG</div>
          </div>
          <div className="max-h-[420px] overflow-y-auto">
            {derived.map((row) => {
              const { session } = row;
              const cancelled = session.status === "cancelled";
              const planned = session.status === "planned";
              const selected = selectedSessionId === session.id;
              const planStatus =
                row.lessonIndex === null
                  ? null
                  : (teaching.lessonPlans[lessonPlanKey(classId, row.lessonIndex)]?.status ??
                    "none");
              const note =
                teaching.sessionNotes[session.id]?.text ??
                (cancelled ? (session.cancel_reason ?? "") : "");
              return (
                <button
                  key={session.id}
                  type="button"
                  aria-expanded={selected}
                  onClick={() => onSelect(session.id)}
                  className={cn(
                    rowGridClassName,
                    "w-full border-b border-line-100 px-4 py-[10px] text-left transition-colors",
                    selected ? "bg-mint-50" : "hover:bg-cream-100",
                  )}
                >
                  <div className="text-[13.5px] font-extrabold text-ink-900">
                    {formatSessionDate(session.session_date)}
                  </div>
                  <div>
                    {planStatus === null ? (
                      <span className="rounded-full bg-cream-200 px-[10px] py-1 text-[12px] font-extrabold text-ink-400">
                        —
                      </span>
                    ) : (
                      <PlanStatusPill status={planStatus} />
                    )}
                  </div>
                  <div className="text-[13.5px] font-extrabold text-ink-700">
                    {cancelled
                      ? "—"
                      : planned
                        ? `${row.eligible} HS`
                        : row.present === null
                          ? "—"
                          : `${row.present}/${row.eligible}`}
                  </div>
                  <div className="text-[13.5px] font-extrabold text-ink-700">
                    {row.average === null || session.status !== "held"
                      ? "—"
                      : row.average.toFixed(1)}
                  </div>
                  <div
                    className={cn(
                      "text-[13.5px] font-extrabold",
                      row.net !== null && row.net < 0 ? "text-coral-600" : "text-ink-900",
                    )}
                  >
                    {cancelled ? "buổi hủy" : row.net === null ? "—" : vnd(row.net)}
                  </div>
                  <div className="overflow-hidden text-[12.5px] text-ink-500 text-ellipsis whitespace-nowrap">
                    {note}
                  </div>
                </button>
              );
            })}
          </div>
        </div>
      </div>
      <p className="px-4 py-[10px] text-[12px] text-ink-400">
        Doanh thu buổi = học phí của học sinh có mặt − 300.000đ chi phí buổi (phòng + trợ giảng).
        Bấm vào buổi để xem giáo án &amp; nhận xét.
      </p>
    </div>
  );
}
