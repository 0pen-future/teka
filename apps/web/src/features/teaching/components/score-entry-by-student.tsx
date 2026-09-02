import * as React from "react";

import {
  HvButton,
  HvIcon,
  HvScoreInput,
  HvStateBlock,
  StatPill,
  type HvScoreInputDirection,
  type ParsedScore,
} from "@/components/hv";
import type { AttendanceRow } from "@/features/attendance";
import { cn } from "@/lib/utils";

import {
  formatAverage,
  formatScore,
  rowStats,
  useRowCells,
  areRowPropsEqual,
  type RowCell,
} from "../hooks/use-row-cells";
import { isGradable, useScoreDraft } from "../hooks/use-score-draft";
import { cellKey } from "../lib/score-entry-summary";
import { ScoreEntryFooter } from "./score-entry-footer";
import { ScoreTableModal } from "./score-table-modal";

export interface ScoreEntryHandle {
  /** Send everything dirty now; resolves `true` once nothing is left unsaved. */
  flush: () => Promise<boolean>;
  /** Drop every unsaved edit, including a pending autosave. */
  discard: () => void;
}

export interface ScoreEntryByStudentProps {
  sessionId: string;
  held: boolean;
  /** Whether the viewer may edit cells — see `SessionExpandRow`'s prop of the same name. */
  canWrite: boolean;
  rosterRows: readonly AttendanceRow[];
  rosterPending: boolean;
  rosterError: boolean;
  sessionLabel: string;
  /** Fires with the unsaved cell count whenever it changes. */
  onDirtyChange?: (dirtyCount: number, invalidCount: number) => void;
}

interface StudentRowProps {
  studentId: string;
  name: string;
  displayNote: string | null;
  late: boolean;
  open: boolean;
  editable: boolean;
  cells: RowCell[];
  scored: number;
  average: number | null;
  onToggle: (studentId: string) => void;
  onRawChange: (key: string, raw: string) => void;
  onCommit: (key: string, parsed: ParsedScore) => void;
  onNavigate: (key: string, direction: HvScoreInputDirection) => void;
  registerInput: (key: string, element: HTMLInputElement | null) => void;
}

/**
 * One accordion row. Memoised with `areRowPropsEqual`, so typing in one
 * student's cell re-renders that row only.
 */
const StudentRow = React.memo(function StudentRow({
  studentId,
  name,
  displayNote,
  late,
  open,
  editable,
  cells,
  scored,
  average,
  onToggle,
  onRawChange,
  onCommit,
  onNavigate,
  registerInput,
}: StudentRowProps) {
  const groupId = `score-entry-${studentId}`;
  return (
    <li className="rounded-[var(--radius-md)] bg-cream-50">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={groupId}
        onClick={() => onToggle(studentId)}
        className="flex min-h-14 w-full items-center gap-2.5 rounded-[var(--radius-md)] px-3 text-left transition-colors hover:bg-cream-100 focus-visible:ring-4 focus-visible:outline-none"
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13.5px] font-bold text-ink-900">{name}</span>
          {displayNote || late ? (
            <span className="block truncate text-[12px] text-ink-400">
              {[displayNote, late ? "Đi muộn" : null].filter(Boolean).join(" · ")}
            </span>
          ) : null}
        </span>
        {average !== null ? <StatPill kind="star" value={formatAverage(average)} /> : null}
        <span className="text-[12.5px] font-bold text-ink-400 tabular-nums">
          {scored}/{cells.length}
        </span>
        <HvIcon
          name="chevron-down"
          size={18}
          className={cn("text-ink-400 transition-transform", open && "rotate-180")}
        />
      </button>
      {open ? (
        <div
          id={groupId}
          role="group"
          aria-label={`Điểm của ${name}`}
          className="grid grid-cols-2 gap-2 px-3 pt-1 pb-3 sm:grid-cols-3"
        >
          {cells.map((cell) => (
            <div key={cell.key} className="flex flex-col gap-1">
              <span className="truncate text-[12px] font-bold text-ink-500">
                {cell.componentName}
              </span>
              {editable ? (
                <HvScoreInput
                  ref={(element) => registerInput(cell.key, element)}
                  size="sm"
                  aria-label={`Điểm ${cell.componentName} ${name}`}
                  value={cell.raw}
                  state={cell.state}
                  onChange={(raw) => onRawChange(cell.key, raw)}
                  onCommit={(parsed) => onCommit(cell.key, parsed)}
                  onNavigate={(direction) => onNavigate(cell.key, direction)}
                />
              ) : (
                <span className="flex min-h-11 items-center justify-center rounded-[var(--radius-md)] bg-cream-200 text-[length:var(--text-md)] font-semibold text-ink-700 tabular-nums">
                  {formatScore(cell.value)}
                </span>
              )}
            </div>
          ))}
        </div>
      ) : null}
    </li>
  );
}, areRowPropsEqual);

/**
 * Scores tab body for a class configured with score components: one
 * accordion row per present/late student holding that student's cells,
 * absent students folded into a read-only "Vắng" group, and a sticky
 * progress footer. Draft and autosave live in `useScoreDraft`; the handle
 * lets the panel flush or discard before it unmounts this component.
 */
export const ScoreEntryByStudent = React.forwardRef<ScoreEntryHandle, ScoreEntryByStudentProps>(
  function ScoreEntryByStudent(
    {
      sessionId,
      held,
      canWrite,
      rosterRows,
      rosterPending,
      rosterError,
      sessionLabel,
      onDirtyChange,
    },
    ref,
  ) {
    const draft = useScoreDraft(sessionId, { rosterRows, canWrite, held, sessionLabel });
    const { components, cells, summary, editableStudentIds, setRaw, commit, flush, discard } =
      draft;

    React.useImperativeHandle(ref, () => ({ flush, discard }), [flush, discard]);

    const { dirtyCount, invalidCount } = summary;
    React.useEffect(() => {
      onDirtyChange?.(dirtyCount, invalidCount);
    }, [dirtyCount, invalidCount, onDirtyChange]);

    // `undefined` = "not chosen yet": the first gradable row opens by default
    // once the roster is in, so grading can start without an extra tap.
    const [openStudent, setOpenStudent] = React.useState<string | null | undefined>(undefined);
    const effectiveOpen =
      openStudent === undefined
        ? (editableStudentIds[0] ?? rosterRows[0]?.student_id ?? null)
        : openStudent;
    const toggleStudent = React.useCallback((studentId: string) => {
      setOpenStudent((current) => (current === studentId ? null : studentId));
    }, []);

    const inputsRef = React.useRef(new Map<string, HTMLInputElement>());
    const registerInput = React.useCallback((key: string, element: HTMLInputElement | null) => {
      if (element) inputsRef.current.set(key, element);
      else inputsRef.current.delete(key);
    }, []);
    const [pendingFocus, setPendingFocus] = React.useState<string | null>(null);
    React.useEffect(() => {
      if (!pendingFocus) return;
      const element = inputsRef.current.get(pendingFocus);
      if (element) {
        element.focus();
        element.select();
        setPendingFocus(null);
      }
    }, [pendingFocus, effectiveOpen]);

    const componentIds = React.useMemo(() => components.map((c) => c.id), [components]);
    const navigate = React.useCallback(
      (key: string, direction: HvScoreInputDirection) => {
        const [studentId = "", componentId = ""] = key.split("#");
        const studentIndex = editableStudentIds.indexOf(studentId);
        const componentIndex = componentIds.indexOf(componentId);
        if (studentIndex < 0 || componentIndex < 0) return;
        const step = direction === "down" ? 1 : -1;
        const nextComponent = componentIndex + step;
        if (nextComponent >= 0 && nextComponent < componentIds.length) {
          inputsRef.current.get(cellKey(studentId, componentIds[nextComponent]!))?.focus();
          return;
        }
        const nextStudent = editableStudentIds[studentIndex + step];
        if (!nextStudent) return;
        const target = cellKey(nextStudent, componentIds[step > 0 ? 0 : componentIds.length - 1]!);
        setOpenStudent(nextStudent);
        setPendingFocus(target);
      },
      [editableStudentIds, componentIds],
    );

    const rowCells = useRowCells(rosterRows, components, cells);
    const [tableOpen, setTableOpen] = React.useState(false);

    const gradableRows = rosterRows.filter(
      (row) => held && (row.status === "present" || row.status === "late"),
    );
    const absentRows = rosterRows.filter(
      (row) => held && (row.status === "absent" || row.status === "excused"),
    );
    // Not held: every row is listed read-only in the main list.
    const listedRows = held ? gradableRows : rosterRows;

    const intro = !held
      ? "Chấm điểm sau khi buổi diễn ra."
      : canWrite
        ? "Chấm điểm từng đầu điểm (0–10), tự lưu khi rời ô — điểm thành phần chưa vào báo cáo phụ huynh."
        : "Chỉ xem — điểm thành phần chưa vào báo cáo phụ huynh.";

    return (
      <div className="flex flex-col">
        <div className="mb-2 flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5">
          <p className="text-[12px] text-ink-400">{intro}</p>
          {held && canWrite && components.length > 0 ? (
            <HvButton
              type="button"
              variant="secondary"
              size="sm"
              icon={<HvIcon name="table" />}
              onClick={() => setTableOpen(true)}
            >
              Mở bảng đầy đủ
            </HvButton>
          ) : null}
        </div>
        {rosterPending || draft.isLoading ? (
          <HvStateBlock state="loading" compact title="Đang tải điểm thành phần…" />
        ) : rosterError || draft.isError ? (
          <HvStateBlock state="error" compact title="Không tải được điểm thành phần." />
        ) : (
          <div className="flex max-h-[min(60dvh,520px)] flex-col overflow-y-auto">
            <ul aria-label="Học sinh" className="flex flex-col gap-1.5">
              {listedRows.map((row) => {
                const rowCellList = rowCells.get(row.student_id) ?? [];
                const { scored, average } = rowStats(rowCellList);
                return (
                  <StudentRow
                    key={row.student_id}
                    studentId={row.student_id}
                    name={row.student_name}
                    displayNote={row.display_note}
                    late={row.status === "late"}
                    open={effectiveOpen === row.student_id}
                    editable={isGradable(row, held, canWrite)}
                    cells={rowCellList}
                    scored={scored}
                    average={average}
                    onToggle={toggleStudent}
                    onRawChange={setRaw}
                    onCommit={commit}
                    onNavigate={navigate}
                    registerInput={registerInput}
                  />
                );
              })}
            </ul>
            {absentRows.length > 0 ? (
              <details className="mt-2">
                <summary className="flex min-h-11 cursor-pointer items-center rounded-[var(--radius-md)] px-3 text-[13px] font-bold text-ink-500 hover:bg-cream-100">
                  Vắng ({absentRows.length})
                </summary>
                <ul className="flex flex-col gap-1 px-3 pb-2">
                  {absentRows.map((row) => {
                    const stored = (rowCells.get(row.student_id) ?? []).filter(
                      (c) => c.value !== null,
                    );
                    return (
                      <li
                        key={row.student_id}
                        className="flex flex-wrap items-center gap-x-2.5 gap-y-1 py-1.5"
                      >
                        <span className="text-[13.5px] font-bold text-ink-700">
                          {row.student_name}
                        </span>
                        <span
                          className={cn(
                            "rounded-full px-2.5 py-[3px] text-[12px] font-extrabold",
                            row.status === "excused"
                              ? "bg-sky-50 text-sky-600"
                              : "bg-coral-100 text-coral-600",
                          )}
                        >
                          {row.status === "excused" ? "Có phép" : "Vắng"}
                        </span>
                        {stored.length > 0 ? (
                          <span className="basis-full text-[12.5px] text-ink-500 tabular-nums">
                            {stored
                              .map((c) => `${c.componentName} ${formatScore(c.value)}`)
                              .join(" · ")}
                          </span>
                        ) : null}
                      </li>
                    );
                  })}
                </ul>
              </details>
            ) : null}
            {held && canWrite ? (
              <ScoreEntryFooter
                scoredStudents={summary.scoredStudents}
                total={summary.total}
                dirtyCount={summary.dirtyCount}
                invalidCount={summary.invalidCount}
                isSaving={draft.isSaving}
                onSave={() => void flush()}
              />
            ) : null}
          </div>
        )}
        {held && canWrite ? (
          <ScoreTableModal
            open={tableOpen}
            onOpenChange={setTableOpen}
            draft={draft}
            rosterRows={rosterRows}
            sessionLabel={sessionLabel}
          />
        ) : null}
      </div>
    );
  },
);
