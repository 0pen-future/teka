import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";

import { hvToast } from "@/components/hv";
import { toApiError } from "@/lib/api/errors";

import { planAction, putCurriculum, putMarks, putNote, savePlan } from "../api/teaching-api";
import type {
  MarkEntryInput,
  MarkResponse,
  MonthMarksResponse,
  PlanActionName,
  PlanResponse,
  PutCurriculumInput,
  SavePlanInput,
} from "../schemas/teaching-schemas";
import { teachingKeys } from "./teaching-keys";

/**
 * Mutations for the teaching endpoints. Reads stay warm through direct cache
 * writes from the response (the server echoes the post-write state), so the
 * classbook keeps rendering its just-saved data without a refetch flash. A
 * conflict (409 — someone else moved the plan's status first) or validation
 * reject invalidates and surfaces the repo's standard danger toast instead of
 * silently keeping the stale view.
 */

function surfaceError(queryClient: QueryClient, classId: string, err: unknown): void {
  const apiError = toApiError(err);
  // Refetch everything for the class: whatever the server refused, the local
  // view may be stale (the usual cause of a 409).
  void queryClient.invalidateQueries({ queryKey: teachingKeys.curriculum(classId) });
  void queryClient.invalidateQueries({ queryKey: teachingKeys.plans(classId) });
  hvToast(
    apiError.status === 409
      ? "Trạng thái giáo án đã thay đổi — đã tải lại, vui lòng thử lại"
      : "Không lưu được — vui lòng thử lại",
    { variant: "danger" },
  );
}

function upsertPlanInCache(queryClient: QueryClient, classId: string, plan: PlanResponse): void {
  queryClient.setQueryData<PlanResponse[]>(teachingKeys.plans(classId), (plans) => {
    const rest = (plans ?? []).filter((row) => row.lesson_index !== plan.lesson_index);
    return [...rest, plan].sort((a, b) => a.lesson_index - b.lesson_index);
  });
}

export function useSaveCurriculum(classId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: PutCurriculumInput) => putCurriculum(classId, input),
    onSuccess: (curriculum) => {
      queryClient.setQueryData(teachingKeys.curriculum(classId), curriculum);
    },
    onError: (err) => surfaceError(queryClient, classId, err),
  });
}

export function useSavePlan(classId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: { lessonIndex: number; input: SavePlanInput }) =>
      savePlan(classId, vars.lessonIndex, vars.input),
    onSuccess: (plan) => upsertPlanInCache(queryClient, classId, plan),
    onError: (err) => surfaceError(queryClient, classId, err),
  });
}

export function usePlanAction(classId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: { lessonIndex: number; action: PlanActionName; comment?: string }) =>
      planAction(classId, vars.lessonIndex, vars.action, vars.comment),
    onSuccess: (plan) => {
      upsertPlanInCache(queryClient, classId, plan);
      // The owner's review queue and nav dot count pending plans; any action
      // can add to or drain that set.
      void queryClient.invalidateQueries({ queryKey: teachingKeys.reviewQueue() });
    },
    onError: (err) => surfaceError(queryClient, classId, err),
  });
}

/** Merge one saved note into the month batch's cached wire response. */
function writeNoteToCache(
  queryClient: QueryClient,
  classId: string,
  month: string,
  sessionId: string,
  body: string,
): void {
  queryClient.setQueryData<MonthMarksResponse>(teachingKeys.marks(classId, month), (data) => {
    if (!data) {
      return data;
    }
    const rest = data.session_notes.filter((note) => note.session_id !== sessionId);
    return {
      ...data,
      // An empty body means the server deleted the row — drop it here too.
      session_notes: body.trim() === "" ? rest : [...rest, { session_id: sessionId, body }],
    };
  });
}

/**
 * Save/delete a session's whole-class note. Writes the cache optimistically so
 * the panel's "Đã lưu ✓" state and the table's note column update in the same
 * render the user saves in — matching the old synchronous store.
 */
export function useSaveSessionNote(classId: string, month: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: { sessionId: string; body: string }) => putNote(vars.sessionId, vars.body),
    onMutate: async (vars) => {
      // Settle any in-flight month read first, or its response would land on
      // top of the optimistic write and briefly resurrect the old note.
      await queryClient.cancelQueries({ queryKey: teachingKeys.marks(classId, month) });
      writeNoteToCache(queryClient, classId, month, vars.sessionId, vars.body);
    },
    onSuccess: (note) => writeNoteToCache(queryClient, classId, month, note.session_id, note.body),
    onError: () => {
      void queryClient.invalidateQueries({ queryKey: teachingKeys.classMarks(classId) });
      hvToast("Không lưu được nhận xét — vui lòng thử lại", { variant: "danger" });
    },
  });
}

/** Apply one tri-state entry to a cached wire mark row set (mirrors the server's merge). */
function applyEntries(rows: MarkResponse[], sessionId: string, entries: MarkEntryInput[]) {
  const bySessionStudent = new Map(
    rows.map((row) => [`${row.session_id}#${row.student_id}`, { ...row }]),
  );
  for (const entry of entries) {
    const key = `${sessionId}#${entry.student_id}`;
    const row = bySessionStudent.get(key) ?? {
      session_id: sessionId,
      student_id: entry.student_id,
      score: null,
      personal_note: null,
    };
    if ("score" in entry) {
      row.score = entry.score ?? null;
    }
    if ("personal_note" in entry) {
      row.personal_note = entry.personal_note ?? null;
    }
    if (row.score === null && row.personal_note === null) {
      bySessionStudent.delete(key);
    } else {
      bySessionStudent.set(key, row);
    }
  }
  return [...bySessionStudent.values()];
}

/**
 * Save a batch of score/personal-note entries for one session (tri-state per
 * field — see `MarkEntryInput`). Optimistic like the note mutation; on success
 * the session's cached rows are replaced with the server's post-write set.
 */
export function useSaveMarks(classId: string, month: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: { sessionId: string; entries: MarkEntryInput[] }) =>
      putMarks(vars.sessionId, vars.entries),
    onMutate: async (vars) => {
      // Same in-flight-read guard as the note mutation.
      await queryClient.cancelQueries({ queryKey: teachingKeys.marks(classId, month) });
      queryClient.setQueryData<MonthMarksResponse>(
        teachingKeys.marks(classId, month),
        (data) =>
          data && { ...data, marks: applyEntries(data.marks, vars.sessionId, vars.entries) },
      );
    },
    onSuccess: (sessionRows, vars) => {
      queryClient.setQueryData<MonthMarksResponse>(
        teachingKeys.marks(classId, month),
        (data) =>
          data && {
            ...data,
            marks: [
              ...data.marks.filter((row) => row.session_id !== vars.sessionId),
              ...sessionRows,
            ],
          },
      );
    },
    onError: () => {
      void queryClient.invalidateQueries({ queryKey: teachingKeys.classMarks(classId) });
      hvToast("Không lưu được điểm — vui lòng thử lại", { variant: "danger" });
    },
  });
}
