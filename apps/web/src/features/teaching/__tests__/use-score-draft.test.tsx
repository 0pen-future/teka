import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import type { AttendanceRow } from "@/features/attendance";
import { useAuthStore } from "@/features/auth";
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { signInAs, testPrimaryTeacher } from "@/test/utils";

import { useScoreDraft } from "../hooks/use-score-draft";
import { cellKey } from "../lib/score-entry-summary";
import {
  getTeachingApiStore,
  resetTeachingApiStore,
  seedComponentScore,
  seedScoreComponents,
  seedTeachingSession,
  teachingHandlers,
} from "./teaching-handlers";

const CLASS_ID = "class-draft";
const SESSION_ID = "session-draft";
const QUIZ = { id: "comp-quiz", name: "15 phút", position: 1 };
const ORAL = { id: "comp-oral", name: "Miệng", position: 2 };

function row(studentId: string, status: AttendanceRow["status"]): AttendanceRow {
  return {
    student_id: studentId,
    student_name: studentId,
    display_note: null,
    enrollment_id: `enr-${studentId}`,
    status,
    billable: status === "present" || status === "late",
    note: null,
  };
}

const ROWS = [row("an", "present"), row("binh", "late"), row("chi", "absent")];
const AN_QUIZ = cellKey("an", QUIZ.id);
const AN_ORAL = cellKey("an", ORAL.id);
const BINH_QUIZ = cellKey("binh", QUIZ.id);

interface PutCall {
  entries: { student_id: string; component_id: string; score: number | null }[];
}

/** Records every PUT body; `release` holds each response until called. */
function trackPuts(options: { hold?: boolean; status?: number } = {}) {
  const calls: PutCall[] = [];
  const releases: (() => void)[] = [];
  server.use(
    http.put(`${API_URL}/sessions/:id/scores`, async ({ params, request }) => {
      const entries = (await request.json()) as PutCall["entries"];
      calls.push({ entries });
      if (options.hold) {
        await new Promise<void>((resolve) => releases.push(resolve));
      }
      if (options.status) {
        return HttpResponse.json(fail("INTERNAL", "boom"), { status: options.status });
      }
      const store = getTeachingApiStore();
      for (const entry of entries) {
        const key = `${params.id as string}#${entry.student_id}#${entry.component_id}`;
        if (entry.score === null) store.componentScores.delete(key);
        else store.componentScores.set(key, entry.score);
      }
      return HttpResponse.json(
        ok(
          [...store.componentScores.entries()]
            .filter(([key]) => key.startsWith(`${params.id as string}#`))
            .map(([key, score]) => {
              const [, student_id, component_id] = key.split("#");
              return { student_id, component_id, score };
            }),
        ),
      );
    }),
  );
  return { calls, release: () => releases.shift()?.() };
}

function renderDraft(overrides: Partial<Parameters<typeof useScoreDraft>[1]> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return renderHook(
    () =>
      useScoreDraft(SESSION_ID, {
        rosterRows: ROWS,
        canWrite: true,
        held: true,
        sessionLabel: "Th 4, 05/08",
        ...overrides,
      }),
    { wrapper },
  );
}

async function renderLoadedDraft(overrides?: Partial<Parameters<typeof useScoreDraft>[1]>) {
  const hook = renderDraft(overrides);
  await waitFor(() => expect(hook.result.current.isLoading).toBe(false));
  return hook;
}

beforeEach(() => {
  resetTeachingApiStore();
  server.use(...teachingHandlers);
  seedTeachingSession({ id: SESSION_ID, class_id: CLASS_ID, session_date: "2026-08-05" });
  seedScoreComponents(CLASS_ID, [ORAL, QUIZ]);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("useScoreDraft", () => {
  it("exposes sorted components and cells only for gradable students in the summary", async () => {
    seedComponentScore(SESSION_ID, "chi", QUIZ.id, 4);
    const { result } = await renderLoadedDraft();

    expect(result.current.components.map((component) => component.id)).toEqual([QUIZ.id, ORAL.id]);
    expect(result.current.editableStudentIds).toEqual(["an", "binh"]);
    expect(result.current.summary).toMatchObject({
      scoredStudents: 0,
      total: 2,
      dirtyCount: 0,
      invalidCount: 0,
    });
    // Absent rows still get cells (read-only display) but never count.
    expect(result.current.cells.get(cellKey("chi", QUIZ.id))).toMatchObject({
      raw: "4",
      server: 4,
      state: "idle",
    });
  });

  it("batches cells committed inside the delay into one PUT and flashes them saved", async () => {
    const { calls } = trackPuts();
    const { result } = await renderLoadedDraft();

    act(() => {
      result.current.setRaw(AN_QUIZ, "7,5");
      result.current.commit(AN_QUIZ, 7.5);
      result.current.setRaw(AN_ORAL, "9");
      result.current.commit(AN_ORAL, 9);
    });
    expect(result.current.isDirty).toBe(true);
    expect(result.current.summary.dirtyCount).toBe(2);
    expect(result.current.cells.get(AN_QUIZ)).toMatchObject({ raw: "7,5", state: "dirty" });

    await waitFor(() => expect(calls).toHaveLength(1), { timeout: 3000 });
    expect(calls[0]!.entries).toEqual([
      { student_id: "an", component_id: QUIZ.id, score: 7.5 },
      { student_id: "an", component_id: ORAL.id, score: 9 },
    ]);
    await waitFor(() =>
      expect(result.current.cells.get(AN_QUIZ)).toMatchObject({
        raw: "7.5",
        server: 7.5,
        state: "saved",
      }),
    );
    expect(result.current.isDirty).toBe(false);
    expect(result.current.summary.scoredStudents).toBe(1);
    await waitFor(() => expect(result.current.cells.get(AN_QUIZ)?.state).toBe("idle"), {
      timeout: 3000,
    });
  });

  it("does not resend a cell reverted to its server value inside the delay", async () => {
    const { calls } = trackPuts();
    const { result } = await renderLoadedDraft();

    act(() => {
      result.current.setRaw(AN_QUIZ, "6");
      result.current.commit(AN_QUIZ, 6);
    });
    act(() => {
      result.current.setRaw(AN_QUIZ, "");
      result.current.commit(AN_QUIZ, null);
    });
    expect(result.current.isDirty).toBe(false);

    await new Promise((resolve) => setTimeout(resolve, 1000));
    expect(calls).toHaveLength(0);
  });

  it("holds an invalid cell out of the payload until it parses", async () => {
    const { calls } = trackPuts();
    const { result } = await renderLoadedDraft();

    act(() => {
      result.current.setRaw(AN_QUIZ, "abc");
      result.current.commit(AN_QUIZ, "invalid");
    });
    expect(result.current.cells.get(AN_QUIZ)?.state).toBe("invalid");
    expect(result.current.summary).toMatchObject({ dirtyCount: 1, invalidCount: 1 });

    // Nothing valid to send, but the invalid cell was not saved either:
    // callers must not treat this flush as "everything is persisted".
    expect(await result.current.flush()).toBe(false);
    expect(calls).toHaveLength(0);
  });

  it("commits a cell still being edited before flushing", async () => {
    const { calls } = trackPuts();
    const { result } = await renderLoadedDraft();

    // No commit: the input was unmounted before its blur (modal closed on Escape).
    act(() => {
      result.current.setRaw(AN_QUIZ, "8");
    });
    let saved = false;
    await act(async () => {
      saved = await result.current.flush();
    });
    expect(saved).toBe(true);
    expect(calls).toHaveLength(1);
    expect(calls[0]!.entries).toEqual([expect.objectContaining({ score: 8 })]);
  });

  it("re-schedules cells committed while a save is in flight, keeping newer drafts", async () => {
    const { calls, release } = trackPuts({ hold: true });
    const { result } = await renderLoadedDraft();

    act(() => {
      result.current.setRaw(AN_QUIZ, "7");
      result.current.commit(AN_QUIZ, 7);
    });
    let flushed!: Promise<boolean>;
    act(() => {
      flushed = result.current.flush();
    });
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(result.current.isSaving).toBe(true);

    // Two edits mid-flight: a new cell, and the in-flight cell retyped.
    act(() => {
      result.current.setRaw(BINH_QUIZ, "5");
      result.current.commit(BINH_QUIZ, 5);
      result.current.setRaw(AN_QUIZ, "8");
      result.current.commit(AN_QUIZ, 8);
    });
    release();
    await expect(flushed).resolves.toBe(true);
    // The echo (7) must not clobber the newer draft (8).
    expect(result.current.cells.get(AN_QUIZ)).toMatchObject({ raw: "8", state: "dirty" });

    await waitFor(() => expect(calls).toHaveLength(2), { timeout: 3000 });
    expect(calls[1]!.entries).toHaveLength(2);
    expect(calls[1]!.entries).toEqual(
      expect.arrayContaining([
        { student_id: "binh", component_id: QUIZ.id, score: 5 },
        { student_id: "an", component_id: QUIZ.id, score: 8 },
      ]),
    );
    release();
    await waitFor(() => expect(result.current.isDirty).toBe(false));
  });

  it("keeps cells dirty after a failed save and lets flush report the failure", async () => {
    const { calls } = trackPuts({ status: 500 });
    const { result } = await renderLoadedDraft();

    act(() => {
      result.current.setRaw(AN_QUIZ, "7");
      result.current.commit(AN_QUIZ, 7);
    });
    let flushed!: Promise<boolean>;
    act(() => {
      flushed = result.current.flush();
    });
    await expect(flushed).resolves.toBe(false);
    expect(calls).toHaveLength(1);
    expect(result.current.cells.get(AN_QUIZ)).toMatchObject({ raw: "7", state: "dirty" });

    // No automatic retry: the teacher decides when to try again.
    await new Promise((resolve) => setTimeout(resolve, 1000));
    expect(calls).toHaveLength(1);
  });

  it("discard drops pending edits without a request", async () => {
    const { calls } = trackPuts();
    const { result } = await renderLoadedDraft();

    act(() => {
      result.current.setRaw(AN_QUIZ, "7");
      result.current.commit(AN_QUIZ, 7);
    });
    act(() => {
      result.current.discard();
    });
    expect(result.current.isDirty).toBe(false);
    expect(result.current.cells.get(AN_QUIZ)).toMatchObject({ raw: "", state: "idle" });

    await new Promise((resolve) => setTimeout(resolve, 1000));
    expect(calls).toHaveLength(0);
  });

  it("marks nobody editable when the session is not held or the viewer cannot write", async () => {
    const planned = await renderLoadedDraft({ held: false });
    expect(planned.result.current.editableStudentIds).toEqual([]);
    expect(planned.result.current.summary.total).toBe(0);

    const readOnly = await renderLoadedDraft({ canWrite: false });
    expect(readOnly.result.current.editableStudentIds).toEqual([]);
  });
});
