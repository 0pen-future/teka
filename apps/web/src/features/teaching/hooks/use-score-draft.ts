import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { hvToast, parseScoreInput, type ParsedScore } from "@/components/hv";
import type { AttendanceRow } from "@/features/attendance";

import {
  cellKey,
  summarize,
  type ScoreCell,
  type ScoreEntrySummary,
} from "../lib/score-entry-summary";
import type { ClassScoreComponent, PutSessionScoreEntryInput } from "../schemas/teaching-schemas";
import { useSaveSessionScores, useSessionScores } from "./use-component-scores";
import { useDebouncedSave } from "./use-debounced-save";

/** Pause after leaving a cell before the dirty set is sent. */
export const SCORE_AUTOSAVE_DELAY_MS = 800;
/** How long a just-saved cell keeps its "saved" ring. */
export const SCORE_SAVED_FLASH_MS = 1500;

export interface UseScoreDraftOptions {
  rosterRows: readonly AttendanceRow[];
  /** Whether the viewer may edit — see `SessionExpandRow`'s prop of the same name. */
  canWrite: boolean;
  held: boolean;
  /** Session label for the success toast (e.g. "Th 4, 05/08"). */
  sessionLabel: string;
}

/**
 * A draft cell: `editing` while the text is still being typed, `dirty` once
 * committed (blur/Enter) and readable, `invalid` once committed and not.
 * Only `dirty` cells go into a payload.
 */
interface DraftCell {
  raw: string;
  state: "editing" | "dirty" | "invalid";
}

/** Present and late students can be graded; absent/excused cells stay read-only. */
export function isGradable(row: AttendanceRow, held: boolean, canWrite: boolean): boolean {
  return held && canWrite && (row.status === "present" || row.status === "late");
}

export interface ScoreDraft {
  components: ClassScoreComponent[];
  cells: ReadonlyMap<string, ScoreCell>;
  summary: ScoreEntrySummary;
  /** Roster order, gradable students only — keyboard navigation walks this list. */
  editableStudentIds: string[];
  isDirty: boolean;
  isSaving: boolean;
  isLoading: boolean;
  isError: boolean;
  setRaw: (key: string, raw: string) => void;
  commit: (key: string, parsed: ParsedScore) => void;
  /** Send everything dirty now. Resolves `true` once nothing is left unsaved. */
  flush: () => Promise<boolean>;
  /** Drop every unsaved edit, including a pending autosave. */
  discard: () => void;
}

/**
 * Draft + save orchestration for one session's component scores, shared by
 * the by-student panel and the full table. Cells autosave as a whole-dirty-set
 * snapshot `SCORE_AUTOSAVE_DELAY_MS` after a commit: `useDebouncedSave`
 * replaces its pending payload, so scheduling only the blurred cell would
 * drop the one before it. TanStack `mutate` does not queue either, so a
 * commit while a PUT is in flight waits for it and is re-scheduled from
 * whatever is still dirty once it settles.
 */
export function useScoreDraft(sessionId: string, options: UseScoreDraftOptions): ScoreDraft {
  const { rosterRows, canWrite, held, sessionLabel } = options;
  const scoresQuery = useSessionScores(sessionId);
  const saveMutation = useSaveSessionScores(sessionId);

  const components = useMemo(
    () => [...(scoresQuery.data?.components ?? [])].sort((a, b) => a.position - b.position),
    [scoresQuery.data],
  );

  const serverByKey = useMemo(() => {
    const map = new Map<string, number>();
    for (const entry of scoresQuery.data?.scores ?? []) {
      if (entry.score !== null) {
        map.set(cellKey(entry.student_id, entry.component_id), entry.score);
      }
    }
    return map;
  }, [scoresQuery.data]);
  const serverRef = useRef(serverByKey);
  useEffect(() => {
    serverRef.current = serverByKey;
  });

  // The draft lives in a ref mirrored into state: save bookkeeping reads it
  // synchronously (inside async continuations) without waiting for a render.
  const [draft, setDraftState] = useState<Record<string, DraftCell>>({});
  const draftRef = useRef(draft);
  const updateDraft = useCallback(
    (updater: (current: Record<string, DraftCell>) => Record<string, DraftCell>) => {
      const next = updater(draftRef.current);
      if (next === draftRef.current) return;
      draftRef.current = next;
      setDraftState(next);
    },
    [],
  );

  const [savedKeys, setSavedKeys] = useState<ReadonlySet<string>>(() => new Set());
  const savedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(
    () => () => {
      if (savedTimerRef.current !== null) clearTimeout(savedTimerRef.current);
    },
    [],
  );
  const flashSaved = useCallback((keys: string[]) => {
    setSavedKeys(new Set(keys));
    if (savedTimerRef.current !== null) clearTimeout(savedTimerRef.current);
    savedTimerRef.current = setTimeout(() => {
      savedTimerRef.current = null;
      setSavedKeys(new Set());
    }, SCORE_SAVED_FLASH_MS);
  }, []);

  const buildPayload = useCallback((): PutSessionScoreEntryInput[] => {
    const entries: PutSessionScoreEntryInput[] = [];
    for (const [key, cell] of Object.entries(draftRef.current)) {
      if (cell.state !== "dirty") continue;
      const [studentId, componentId] = key.split("#");
      if (!studentId || !componentId) continue;
      const parsed = parseScoreInput(cell.raw);
      if (parsed === "invalid") continue;
      // An emptied cell sends `null` to clear it server-side.
      entries.push({ student_id: studentId, component_id: componentId, score: parsed });
    }
    return entries;
  }, []);

  const flightRef = useRef<Promise<boolean> | null>(null);
  const sendRef = useRef<(entries: PutSessionScoreEntryInput[]) => Promise<boolean>>(() =>
    Promise.resolve(true),
  );
  // The payload is built when the timer fires, not when it is scheduled: a
  // cell reverted to its server value inside the delay must not be resent
  // from a stale snapshot.
  const debounced = useDebouncedSave<void>(() => {
    void sendRef.current(buildPayload());
  }, SCORE_AUTOSAVE_DELAY_MS);

  const { mutateAsync } = saveMutation;
  const send = useCallback(
    async (entries: PutSessionScoreEntryInput[]): Promise<boolean> => {
      if (entries.length === 0) return true;
      if (flightRef.current) {
        // A timer fired mid-flight: try again once the request settles
        // instead of leaving the cells dirty with nothing scheduled.
        debounced.schedule();
        return false;
      }
      // Remember what each cell said when sent: a cell retyped mid-flight
      // keeps its newer draft instead of being cleared by the echo.
      const sentRaw = new Map(
        entries.map((entry) => {
          const key = cellKey(entry.student_id, entry.component_id);
          return [key, draftRef.current[key]?.raw] as const;
        }),
      );
      const flight = (async () => {
        try {
          await mutateAsync(entries);
          updateDraft((current) => {
            const next = { ...current };
            for (const [key, raw] of sentRaw) {
              if (next[key]?.raw === raw) delete next[key];
            }
            return next;
          });
          flashSaved([...sentRaw.keys()]);
          hvToast(`Đã lưu điểm thành phần (${entries.length} ô) — buổi ${sessionLabel}`);
          return true;
        } catch {
          // The mutation hook already toasted; cells stay dirty for a retry.
          return false;
        }
      })();
      flightRef.current = flight;
      const saved = await flight;
      flightRef.current = null;
      if (saved && buildPayload().length > 0) debounced.schedule();
      return saved;
    },
    [buildPayload, debounced, flashSaved, mutateAsync, sessionLabel, updateDraft],
  );
  useEffect(() => {
    sendRef.current = send;
  });

  const setRaw = useCallback(
    (key: string, raw: string) => {
      updateDraft((current) => ({ ...current, [key]: { raw, state: "editing" } }));
    },
    [updateDraft],
  );

  const commit = useCallback(
    (key: string, parsed: ParsedScore) => {
      const current = draftRef.current[key];
      if (!current) return;
      if (parsed === "invalid") {
        updateDraft((draft) => ({ ...draft, [key]: { raw: current.raw, state: "invalid" } }));
        return;
      }
      const server = serverRef.current.get(key) ?? null;
      if (parsed === server) {
        // Back to what the server has — nothing to save.
        updateDraft((draft) => {
          const next = { ...draft };
          delete next[key];
          return next;
        });
        return;
      }
      updateDraft((draft) => ({ ...draft, [key]: { raw: current.raw, state: "dirty" } }));
      if (!flightRef.current) debounced.schedule();
    },
    [debounced, updateDraft],
  );

  const flush = useCallback(async (): Promise<boolean> => {
    // A cell whose input was unmounted mid-edit (e.g. the full table closed
    // on Escape) never got its blur commit; settle it here so the text the
    // user typed is either sent or reported as invalid, never dropped.
    for (const [key, cell] of Object.entries(draftRef.current)) {
      if (cell.state === "editing") commit(key, parseScoreInput(cell.raw));
    }
    debounced.cancel();
    if (flightRef.current) {
      const previous = await flightRef.current;
      if (!previous) return false;
    }
    const saved = await send(buildPayload());
    // Invalid cells are never sent, so a flush that leaves one behind did
    // not save everything the user typed and must not report success.
    const invalidLeft = Object.values(draftRef.current).some((cell) => cell.state === "invalid");
    return saved && !invalidLeft;
  }, [buildPayload, commit, debounced, send]);

  const discard = useCallback(() => {
    debounced.cancel();
    updateDraft(() => ({}));
  }, [debounced, updateDraft]);

  const editableStudentIds = useMemo(
    () => rosterRows.filter((row) => isGradable(row, held, canWrite)).map((row) => row.student_id),
    [rosterRows, held, canWrite],
  );

  const cells = useMemo(() => {
    const map = new Map<string, ScoreCell>();
    for (const row of rosterRows) {
      for (const component of components) {
        const key = cellKey(row.student_id, component.id);
        const server = serverByKey.get(key) ?? null;
        const cell = draft[key];
        map.set(key, {
          raw: cell?.raw ?? (server === null ? "" : String(server)),
          server,
          state: cell
            ? cell.state === "invalid"
              ? "invalid"
              : "dirty"
            : savedKeys.has(key)
              ? "saved"
              : "idle",
        });
      }
    }
    return map;
  }, [rosterRows, components, serverByKey, draft, savedKeys]);

  const summary = useMemo(() => {
    const gradable = new Set(editableStudentIds);
    return summarize(
      cells,
      rosterRows.filter((row) => gradable.has(row.student_id)),
      components,
    );
  }, [cells, rosterRows, editableStudentIds, components]);

  return {
    components,
    cells,
    summary,
    editableStudentIds,
    isDirty: Object.keys(draft).length > 0,
    isSaving: saveMutation.isPending,
    isLoading: scoresQuery.isPending,
    isError: scoresQuery.isError,
    setRaw,
    commit,
    flush,
    discard,
  };
}
