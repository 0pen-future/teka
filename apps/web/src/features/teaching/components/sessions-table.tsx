import * as React from "react";

import { ProgressBar } from "@/components/hv";
import { cn, formatSessionDate } from "@/lib/utils";

import { formatLedgerScore, sessionWorkStatus, type SessionDerived } from "../lib/classbook-stats";
import { lessonPlanKey, type Curriculum, type TeachingState } from "../lib/teaching-store";
import { vnd } from "../lib/vnd";
import { PlanStatusPill } from "./plan-status-pill";
import { SessionStatusChip } from "./session-status-chip";

interface SessionsTableProps {
  rows: SessionDerived[];
  classId: string;
  curriculum: Curriculum | undefined;
  lessonPlans: TeachingState["lessonPlans"];
  notes: TeachingState["sessionNotes"];
  /** Students with ≥1 score per session id, from either scoring model. */
  scoredCounts: Record<string, number>;
  selectedId: string | null;
  onSelect: (sessionId: string) => void;
  /** Escape on a row button while one is open. */
  onClose: () => void;
  /** Renders the open session's detail inside the row that follows it. */
  renderExpanded: (row: SessionDerived) => React.ReactNode;
}

function expandRowId(sessionId: string): string {
  return `session-expand-${sessionId}`;
}

const COLUMN_COUNT = 8;

const headClassName =
  "px-3 py-3 text-left text-[11.5px] font-extrabold tracking-[0.3px] text-ink-400 whitespace-nowrap";
const cellClassName = "px-3 py-[9px] align-middle text-[13.5px]";
const muted = <span className="text-ink-300">·</span>;

/**
 * The classbook ledger: one `<tr>` per session, the open session's detail
 * rendered in a full-width row directly below it. The date cell's button
 * carries the disclosure semantics; ↑/↓ walk the rows, Enter/Space toggle,
 * Escape closes. Phones keep BUỔI / CÓ MẶT / VIỆC and fold the rest into
 * the expand row.
 */
export function SessionsTable({
  rows,
  classId,
  curriculum,
  lessonPlans,
  notes,
  scoredCounts,
  selectedId,
  onSelect,
  onClose,
  renderExpanded,
}: SessionsTableProps) {
  const buttonsRef = React.useRef<Map<string, HTMLButtonElement>>(new Map());
  // The roving tabindex follows the last focused row, so Tab out and back
  // lands where the teacher left off rather than on the first row.
  const [focusedId, setFocusedId] = React.useState<string | null>(null);
  const hasRow = (id: string | null) => id !== null && rows.some((r) => r.session.id === id);
  const focusRowId = hasRow(focusedId)
    ? focusedId
    : hasRow(selectedId)
      ? selectedId
      : rows[0]?.session.id;

  // Closing from inside the expand row (footer button, Escape in a field)
  // unmounts the focused element; hand focus back to that row's button.
  const previousSelectedRef = React.useRef(selectedId);
  React.useEffect(() => {
    const previous = previousSelectedRef.current;
    previousSelectedRef.current = selectedId;
    if (previous && selectedId === null && document.activeElement === document.body) {
      buttonsRef.current.get(previous)?.focus();
    }
  }, [selectedId]);

  function handleKeyDown(event: React.KeyboardEvent<HTMLButtonElement>, sessionId: string) {
    const index = rows.findIndex((row) => row.session.id === sessionId);
    if (index === -1) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const next = rows[index + (event.key === "ArrowDown" ? 1 : -1)];
      if (next) buttonsRef.current.get(next.session.id)?.focus();
    } else if (event.key === "Escape" && selectedId) {
      event.preventDefault();
      onClose();
      buttonsRef.current.get(selectedId)?.focus();
    }
  }

  return (
    <div className="overflow-hidden rounded-[var(--radius-lg)] bg-white shadow-soft-md">
      <div className="overflow-x-auto">
        <table className="w-full border-collapse sm:min-w-[960px]">
          <thead>
            <tr className="border-b-[1.5px] border-line-200">
              <th scope="col" className={headClassName}>
                BUỔI
              </th>
              <th scope="col" className={cn(headClassName, "hidden sm:table-cell")}>
                BÀI HỌC
              </th>
              <th scope="col" className={cn(headClassName, "hidden sm:table-cell")}>
                GIÁO ÁN
              </th>
              <th scope="col" className={headClassName}>
                CÓ MẶT
              </th>
              <th scope="col" className={cn(headClassName, "hidden text-right sm:table-cell")}>
                ĐTB
              </th>
              <th scope="col" className={cn(headClassName, "hidden text-right sm:table-cell")}>
                DOANH THU
              </th>
              <th scope="col" className={cn(headClassName, "hidden sm:table-cell")}>
                NHẬN XÉT
              </th>
              <th scope="col" className={cn(headClassName, "hidden sm:table-cell")}>
                CHẤM ĐIỂM
              </th>
              <th scope="col" className={cn(headClassName, "sm:hidden")}>
                VIỆC
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => {
              const { session } = row;
              const held = session.status === "held";
              const cancelled = session.status === "cancelled";
              const planned = session.status === "planned";
              const selected = selectedId === session.id;
              const lessonNumber = row.lessonIndex === null ? null : row.lessonIndex + 1;
              const lessonTitle =
                row.lessonIndex === null ? undefined : curriculum?.lessons[row.lessonIndex];
              const planStatus =
                row.lessonIndex === null
                  ? null
                  : (lessonPlans[lessonPlanKey(classId, row.lessonIndex)]?.status ?? "none");
              const work = sessionWorkStatus(
                row,
                notes[session.id]?.text,
                scoredCounts[session.id] ?? 0,
              );
              const attendancePct =
                held && row.present !== null && row.eligible
                  ? Math.round((row.present / row.eligible) * 100)
                  : 0;
              const dateLabel = formatSessionDate(session.session_date);

              return (
                <React.Fragment key={session.id}>
                  <tr
                    onClick={() => onSelect(session.id)}
                    className={cn(
                      "cursor-pointer border-b border-line-100 transition-colors",
                      selected ? "bg-mint-50" : "hover:bg-cream-100",
                      cancelled && !selected && "text-ink-400",
                    )}
                  >
                    <td className={cellClassName}>
                      <button
                        type="button"
                        ref={(node) => {
                          if (node) buttonsRef.current.set(session.id, node);
                          else buttonsRef.current.delete(session.id);
                        }}
                        onKeyDown={(event) => handleKeyDown(event, session.id)}
                        onFocus={() => setFocusedId(session.id)}
                        tabIndex={focusRowId === session.id ? 0 : -1}
                        aria-expanded={selected}
                        aria-controls={selected ? expandRowId(session.id) : undefined}
                        onClick={(event) => {
                          event.stopPropagation();
                          onSelect(session.id);
                        }}
                        className="-mx-1 flex min-h-8 flex-col rounded-[8px] px-1 text-left focus-visible:ring-4 focus-visible:outline-none"
                      >
                        <span
                          className={cn(
                            "font-extrabold whitespace-nowrap",
                            cancelled ? "text-ink-400" : "text-ink-900",
                          )}
                        >
                          {dateLabel}
                        </span>
                        {selected ? (
                          <span className="text-[11px] font-bold text-mint-600">đang mở</span>
                        ) : lessonNumber !== null ? (
                          <span className="text-[11px] text-ink-400 sm:hidden">
                            Bài {lessonNumber}
                          </span>
                        ) : null}
                      </button>
                    </td>

                    <td className={cn(cellClassName, "hidden max-w-[260px] sm:table-cell")}>
                      {cancelled ? (
                        <span className="text-ink-500 italic">
                          {session.cancel_reason?.trim() ? session.cancel_reason : "Buổi hủy"}
                        </span>
                      ) : lessonNumber === null ? (
                        muted
                      ) : (
                        <span className="block truncate text-ink-700">
                          <span className="font-extrabold text-ink-900">Bài {lessonNumber}</span>
                          {lessonTitle ? ` · ${lessonTitle}` : ""}
                        </span>
                      )}
                    </td>

                    <td className={cn(cellClassName, "hidden sm:table-cell")}>
                      {cancelled ? (
                        <SessionStatusChip tone="coral">Buổi hủy</SessionStatusChip>
                      ) : planStatus === null ? (
                        muted
                      ) : (
                        <PlanStatusPill status={planStatus} />
                      )}
                    </td>

                    <td className={cn(cellClassName, "min-w-[96px]")}>
                      {cancelled ? (
                        muted
                      ) : planned ? (
                        <span className="text-ink-500">{row.eligible ?? 0} dự kiến</span>
                      ) : row.present === null ? (
                        muted
                      ) : (
                        <span className="flex flex-col gap-1">
                          <span className="font-extrabold text-ink-900 tabular-nums">
                            {row.present}/{row.eligible}
                          </span>
                          <ProgressBar
                            value={attendancePct}
                            color={attendancePct < 70 ? "missing" : "mint"}
                            size="sm"
                            aria-label={`Có mặt ${attendancePct}%`}
                            className="w-16"
                          />
                        </span>
                      )}
                    </td>

                    <td
                      className={cn(
                        cellClassName,
                        "hidden text-right font-extrabold text-ink-700 tabular-nums sm:table-cell",
                      )}
                    >
                      {held && row.average !== null ? formatLedgerScore(row.average) : muted}
                    </td>

                    <td
                      className={cn(
                        cellClassName,
                        "hidden text-right font-extrabold tabular-nums sm:table-cell",
                        row.net !== null && row.net < 0 ? "text-coral-600" : "text-ink-900",
                      )}
                    >
                      {held && row.net !== null ? vnd(row.net) : muted}
                    </td>

                    <td className={cn(cellClassName, "hidden sm:table-cell")}>
                      {work.noteChip === "none" ? (
                        muted
                      ) : (
                        <SessionStatusChip tone={work.noteChip === "done" ? "mint" : "sun"}>
                          {work.noteChip === "done" ? "Đã có" : "Chưa có"}
                        </SessionStatusChip>
                      )}
                    </td>

                    <td className={cn(cellClassName, "hidden sm:table-cell")}>
                      {work.scoreChip === "none" ? (
                        muted
                      ) : (
                        <SessionStatusChip tone={work.scoreChip === "done" ? "mint" : "sun"}>
                          {work.scored}/{work.total}
                        </SessionStatusChip>
                      )}
                    </td>

                    <td className={cn(cellClassName, "sm:hidden")}>
                      {cancelled ? (
                        <SessionStatusChip tone="muted">Hủy</SessionStatusChip>
                      ) : planned ? (
                        muted
                      ) : work.noteChip === "missing" ? (
                        <SessionStatusChip tone="sun">Nhận xét</SessionStatusChip>
                      ) : work.scoreChip === "partial" ? (
                        <SessionStatusChip tone="sun">
                          Điểm {work.scored}/{work.total}
                        </SessionStatusChip>
                      ) : (
                        <SessionStatusChip tone="mint">Xong</SessionStatusChip>
                      )}
                    </td>
                  </tr>
                  {selected ? (
                    <tr id={expandRowId(session.id)}>
                      <td colSpan={COLUMN_COUNT + 1} className="p-0">
                        {renderExpanded(row)}
                      </td>
                    </tr>
                  ) : null}
                </React.Fragment>
              );
            })}
          </tbody>
        </table>
      </div>
      <p className="px-4 py-[10px] text-[12px] text-ink-400">
        Doanh thu buổi = học phí của học sinh có mặt − 300.000đ chi phí buổi (phòng + trợ giảng).
        Bấm vào buổi để mở nhận xét, giáo án &amp; điểm.
      </p>
    </div>
  );
}
