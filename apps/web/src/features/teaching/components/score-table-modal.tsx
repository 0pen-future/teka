import * as React from "react";

import {
  HvButton,
  HvModal,
  HvScoreInput,
  type HvScoreInputDirection,
  type ParsedScore,
} from "@/components/hv";
import type { AttendanceRow } from "@/features/attendance";

import {
  areRowPropsEqual,
  formatAverage,
  rowStats,
  useRowCells,
  type RowCell,
} from "../hooks/use-row-cells";
import type { ScoreDraft } from "../hooks/use-score-draft";
import { cellKey } from "../lib/score-entry-summary";
import { ScoreEntryFooter } from "./score-entry-footer";

export interface ScoreTableModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The panel's draft — the table is another view of the same unsaved cells. */
  draft: ScoreDraft;
  rosterRows: readonly AttendanceRow[];
  sessionLabel: string;
}

const NAME_COLUMN = "w-[180px] min-w-[180px]";
const SCORE_COLUMN = "w-[72px] min-w-[72px]";
const HEADER_CELL = "sticky top-0 z-10 bg-white px-1.5 py-2 text-[12px] font-bold text-ink-500";

interface TableRowProps {
  studentId: string;
  name: string;
  displayNote: string | null;
  late: boolean;
  cells: RowCell[];
  average: number | null;
  onRawChange: (key: string, raw: string) => void;
  onCommit: (key: string, parsed: ParsedScore) => void;
  onNavigate: (key: string, direction: HvScoreInputDirection) => void;
  registerInput: (key: string, element: HTMLInputElement | null) => void;
}

/** One gradable student. Memoised like the panel row: typing re-renders one `<tr>`. */
const TableRow = React.memo(function TableRow({
  name,
  displayNote,
  late,
  cells,
  average,
  onRawChange,
  onCommit,
  onNavigate,
  registerInput,
}: TableRowProps) {
  return (
    <tr>
      <th
        scope="row"
        className={`sticky left-0 z-10 bg-white px-2 py-1 text-left font-normal ${NAME_COLUMN}`}
      >
        <span className="block truncate text-[13.5px] font-bold text-ink-900">{name}</span>
        {displayNote || late ? (
          <span className="block truncate text-[12px] text-ink-400">
            {[displayNote, late ? "Đi muộn" : null].filter(Boolean).join(" · ")}
          </span>
        ) : null}
      </th>
      {cells.map((cell) => (
        <td key={cell.key} className={`px-1 py-1 ${SCORE_COLUMN}`}>
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
        </td>
      ))}
      <td
        className={`px-1.5 py-1 text-center text-[13.5px] font-bold text-ink-700 tabular-nums ${SCORE_COLUMN}`}
      >
        {formatAverage(average)}
      </td>
    </tr>
  );
}, areRowPropsEqual);

/**
 * Full students × columns table for a held session, in an xl modal. It owns
 * no draft of its own: cells, autosave and the progress footer all come from
 * the panel's `useScoreDraft`, so closing the table returns to the panel with
 * every unsaved cell intact. Enter/Shift+Enter walk the same column across
 * gradable rows; Tab is left to the browser.
 */
export function ScoreTableModal({
  open,
  onOpenChange,
  draft,
  rosterRows,
  sessionLabel,
}: ScoreTableModalProps) {
  const { components, cells, summary, editableStudentIds, setRaw, commit, flush } = draft;
  const rowCells = useRowCells(rosterRows, components, cells);

  const editableSet = React.useMemo(() => new Set(editableStudentIds), [editableStudentIds]);
  const gradableRows = rosterRows.filter((row) => editableSet.has(row.student_id));
  const absentRows = rosterRows.filter(
    (row) => row.status === "absent" || row.status === "excused",
  );

  const inputsRef = React.useRef(new Map<string, HTMLInputElement>());
  const registerInput = React.useCallback((key: string, element: HTMLInputElement | null) => {
    if (element) inputsRef.current.set(key, element);
    else inputsRef.current.delete(key);
  }, []);
  const navigate = React.useCallback(
    (key: string, direction: HvScoreInputDirection) => {
      const [studentId = "", componentId = ""] = key.split("#");
      const studentIndex = editableStudentIds.indexOf(studentId);
      if (studentIndex < 0) return;
      const nextStudent = editableStudentIds[studentIndex + (direction === "down" ? 1 : -1)];
      if (!nextStudent) return;
      const target = inputsRef.current.get(cellKey(nextStudent, componentId));
      target?.focus();
      target?.select();
    },
    [editableStudentIds],
  );

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      size="xl"
      title={`Bảng điểm — buổi ${sessionLabel}`}
      description="Enter xuống hàng, Shift+Enter lên hàng, Tab sang ô bên phải. Tự lưu khi rời ô; đóng bảng vẫn giữ ô chưa lưu."
      footer={
        <ScoreEntryFooter
          variant="plain"
          scoredStudents={summary.scoredStudents}
          total={summary.total}
          dirtyCount={summary.dirtyCount}
          invalidCount={summary.invalidCount}
          isSaving={draft.isSaving}
          onSave={() => void flush()}
          actions={
            <HvButton
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => onOpenChange(false)}
            >
              Đóng
            </HvButton>
          }
        />
      }
    >
      <table className="w-full border-separate border-spacing-0 text-[13px]">
        <thead>
          <tr>
            <th
              scope="col"
              className={`sticky top-0 left-0 z-20 bg-white px-2 py-2 text-left text-[12px] font-bold text-ink-500 ${NAME_COLUMN}`}
            >
              Học sinh
            </th>
            {components.map((component) => (
              <th
                key={component.id}
                scope="col"
                aria-label={component.name}
                title={component.name}
                className={`${HEADER_CELL} text-center ${SCORE_COLUMN}`}
              >
                <span className="line-clamp-2 break-words">{component.name}</span>
              </th>
            ))}
            <th scope="col" className={`${HEADER_CELL} text-center ${SCORE_COLUMN}`}>
              TB
            </th>
          </tr>
        </thead>
        <tbody>
          {gradableRows.map((row) => {
            const rowCellList = rowCells.get(row.student_id) ?? [];
            return (
              <TableRow
                key={row.student_id}
                studentId={row.student_id}
                name={row.student_name}
                displayNote={row.display_note}
                late={row.status === "late"}
                cells={rowCellList}
                average={rowStats(rowCellList).average}
                onRawChange={setRaw}
                onCommit={commit}
                onNavigate={navigate}
                registerInput={registerInput}
              />
            );
          })}
          {absentRows.length > 0 ? (
            <tr>
              <td
                colSpan={components.length + 2}
                className="sticky left-0 bg-cream-50 px-2 py-2.5 text-[12.5px] text-ink-500"
              >
                <span className="font-bold">Vắng ({absentRows.length}):</span>{" "}
                {absentRows.map((row) => row.student_name).join(", ")}
              </td>
            </tr>
          ) : null}
        </tbody>
      </table>
    </HvModal>
  );
}
