import { useMemo, useState } from "react";

import type { AttendanceRow } from "@/features/attendance";
import { hvToast } from "@/components/hv";
import { cn } from "@/lib/utils";

import { parseScoreInput } from "../lib/classbook-stats";
import { saveButtonActive, saveButtonIdle } from "../lib/save-button-styles";
import { useSaveSessionScores, useSessionScores } from "../hooks/use-component-scores";
import type { PutSessionScoreEntryInput } from "../schemas/teaching-schemas";

interface ComponentScoreGridProps {
  sessionId: string;
  held: boolean;
  rosterRows: readonly AttendanceRow[];
  rosterPending: boolean;
  rosterError: boolean;
  sessionLabel: string;
  /** Whether the viewer may edit cells — see `SessionDetailPanel`'s prop of the same name. */
  canWrite: boolean;
}

function cellKey(studentId: string, componentId: string): string {
  return `${studentId}#${componentId}`;
}

/**
 * Student×component score grid — replaces the general-score block in the
 * scores tab for classes configured with `class_score_components`. Columns
 * come from the session-scores read (already ordered with the class's
 * components), rows from the roster the panel already loaded; a cell is
 * editable under the same held+present rule as the general-score input.
 */
export function ComponentScoreGrid({
  sessionId,
  held,
  rosterRows,
  rosterPending,
  rosterError,
  sessionLabel,
  canWrite,
}: ComponentScoreGridProps) {
  const scoresQuery = useSessionScores(sessionId);
  const saveMutation = useSaveSessionScores(sessionId);
  const [draft, setDraft] = useState<Record<string, string>>({});

  const components = useMemo(
    () => [...(scoresQuery.data?.components ?? [])].sort((a, b) => a.position - b.position),
    [scoresQuery.data],
  );

  const storedByKey = useMemo(() => {
    const map = new Map<string, number>();
    for (const entry of scoresQuery.data?.scores ?? []) {
      if (entry.score !== null) {
        map.set(cellKey(entry.student_id, entry.component_id), entry.score);
      }
    }
    return map;
  }, [scoresQuery.data]);

  const dirty = Object.keys(draft).length > 0;

  // Mirrors `SessionDetailPanel.saveScores`: an explicitly emptied field
  // sends `score: null` to clear the cell (this API supports that, unlike
  // the tri-state general-mark endpoint); unparsable non-empty input is
  // dropped rather than guessed at.
  function saveScores() {
    if (!dirty) {
      return;
    }
    const entries: PutSessionScoreEntryInput[] = [];
    for (const [key, raw] of Object.entries(draft)) {
      const [studentId, componentId] = key.split("#");
      if (!studentId || !componentId) {
        continue;
      }
      if (raw.trim() === "") {
        entries.push({ student_id: studentId, component_id: componentId, score: null });
        continue;
      }
      const score = parseScoreInput(raw);
      if (score !== null) {
        entries.push({ student_id: studentId, component_id: componentId, score });
      }
    }
    if (entries.length === 0) {
      setDraft({});
      return;
    }
    saveMutation.mutate(entries, {
      onSuccess: () => {
        setDraft({});
        hvToast(`Đã lưu điểm thành phần (${entries.length} ô) — buổi ${sessionLabel}`);
      },
    });
  }

  return (
    <>
      <div className="mb-1.5 text-[12px] text-ink-400">
        Chấm điểm từng đầu điểm (0–10) rồi bấm lưu — điểm thành phần chưa vào báo cáo phụ huynh.
      </div>
      {rosterPending || scoresQuery.isPending ? (
        <p className="text-[13px] text-ink-500">Đang tải điểm thành phần…</p>
      ) : rosterError || scoresQuery.isError ? (
        <p className="text-[13px] text-coral-600">Không tải được điểm thành phần.</p>
      ) : (
        <div className="max-h-[280px] overflow-x-auto overflow-y-auto">
          <table className="w-full min-w-max border-separate border-spacing-y-1 text-[13.5px]">
            <thead>
              <tr>
                <th className="sticky left-0 z-10 bg-white px-2 py-1 text-left font-extrabold text-ink-400">
                  Học sinh
                </th>
                {components.map((component) => (
                  <th
                    key={component.id}
                    className="px-2 py-1 text-center font-extrabold text-ink-400"
                  >
                    {component.name}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rosterRows.map((row) => {
                const absent = row.status === "absent" || row.status === "excused";
                const editable = held && row.status === "present" && canWrite;
                return (
                  <tr key={row.student_id} className="hover:bg-cream-100">
                    <td className="sticky left-0 z-10 bg-white px-2 py-1 whitespace-nowrap text-ink-700">
                      <span className="font-bold">{row.student_name}</span>
                    </td>
                    {components.map((component) => {
                      const key = cellKey(row.student_id, component.id);
                      const stored = storedByKey.get(key);
                      return (
                        <td key={component.id} className="px-2 py-1 text-center">
                          {editable ? (
                            <input
                              type="number"
                              min={0}
                              max={10}
                              step={0.5}
                              aria-label={`Điểm ${component.name} ${row.student_name}`}
                              value={draft[key] ?? stored?.toString() ?? ""}
                              onChange={(event) =>
                                setDraft((current) => ({ ...current, [key]: event.target.value }))
                              }
                              className="w-16 rounded-[10px] border-2 border-line-200 px-2 py-[5px] text-center text-[13.5px] font-extrabold text-ink-900 outline-none focus:border-mint-400"
                            />
                          ) : (
                            <span
                              className={cn(
                                "inline-block rounded-full px-2.5 py-[3px] text-[13px] font-extrabold",
                                absent && held
                                  ? "bg-coral-100 text-coral-600"
                                  : "bg-cream-200 text-ink-400",
                              )}
                            >
                              {!held ? "—" : absent ? "Vắng" : "—"}
                            </span>
                          )}
                        </td>
                      );
                    })}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {canWrite ? (
        <div className="mt-2.5 flex items-center gap-2.5">
          <button
            type="button"
            onClick={saveScores}
            className={dirty ? saveButtonActive : saveButtonIdle}
          >
            Lưu điểm thành phần
          </button>
          <span className="text-[12.5px] font-bold text-sun-600">{dirty ? "Chưa lưu" : ""}</span>
        </div>
      ) : null}
    </>
  );
}
