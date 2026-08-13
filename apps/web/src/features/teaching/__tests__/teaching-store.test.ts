import { afterEach, describe, expect, it, vi } from "vitest";

import {
  countPendingPlans,
  getTeachingSnapshot,
  lessonPlanKey,
  personalNoteKey,
  resetTeachingStoreForTests,
  SESSION_COST_VND,
  subscribeTeaching,
  updateTeachingState,
  type TeachingState,
} from "../lib/teaching-store";

const CENTER_A = "center-a";
const CENTER_B = "center-b";

function makePlan(status: TeachingState["lessonPlans"][string]["status"]) {
  return {
    goal: "Ôn tập chương 1",
    activities: ["Khởi động", "Luyện tập"],
    homework: "Bài 1–5 trang 20",
    status,
  };
}

afterEach(() => {
  resetTeachingStoreForTests();
  localStorage.clear();
});

describe("teaching store", () => {
  it("returns an empty state for a center with no saved data", () => {
    const state = getTeachingSnapshot(CENTER_A);
    expect(state.curricula).toEqual({});
    expect(state.lessonPlans).toEqual({});
    expect(state.sessionNotes).toEqual({});
    expect(state.sessionScores).toEqual({});
    expect(state.personalNotes).toEqual({});
  });

  it("keeps snapshot identity stable between reads with no writes", () => {
    expect(getTeachingSnapshot(CENTER_A)).toBe(getTeachingSnapshot(CENTER_A));
  });

  it("round-trips writes through localStorage across a simulated reload", () => {
    updateTeachingState(CENTER_A, (state) => ({
      ...state,
      curricula: { "class-1": { lessons: ["Bài 1", "Bài 2"], currentIndex: 1 } },
      sessionScores: { "session-1": { "student-1": 8.5 } },
      personalNotes: { [personalNoteKey("session-1", "student-1")]: "Tiến bộ rõ" },
    }));

    // Simulated reload: drop the in-memory cache, keep localStorage.
    resetTeachingStoreForTests();

    const state = getTeachingSnapshot(CENTER_A);
    expect(state.curricula["class-1"]).toEqual({ lessons: ["Bài 1", "Bài 2"], currentIndex: 1 });
    expect(state.sessionScores["session-1"]).toEqual({ "student-1": 8.5 });
    expect(state.personalNotes[personalNoteKey("session-1", "student-1")]).toBe("Tiến bộ rõ");
  });

  it("namespaces persistence per center and never leaks across centers", () => {
    updateTeachingState(CENTER_A, (state) => ({
      ...state,
      sessionNotes: { "session-1": { text: "Lớp học sôi nổi" } },
    }));

    expect(localStorage.getItem(`teka.teaching.${CENTER_A}`)).not.toBeNull();
    expect(localStorage.getItem(`teka.teaching.${CENTER_B}`)).toBeNull();
    expect(getTeachingSnapshot(CENTER_B).sessionNotes).toEqual({});
  });

  it("falls back to an empty state on corrupt JSON without throwing", () => {
    localStorage.setItem(`teka.teaching.${CENTER_A}`, "{definitely not json");
    expect(() => getTeachingSnapshot(CENTER_A)).not.toThrow();
    expect(getTeachingSnapshot(CENTER_A).curricula).toEqual({});
  });

  it("falls back to an empty state on schema-mismatched data", () => {
    localStorage.setItem(
      `teka.teaching.${CENTER_A}`,
      JSON.stringify({ version: 1, state: { curricula: { "class-1": { lessons: "oops" } } } }),
    );
    expect(getTeachingSnapshot(CENTER_A).curricula).toEqual({});
  });

  it("notifies subscribers and swaps snapshot identity on update", () => {
    const before = getTeachingSnapshot(CENTER_A);
    const listener = vi.fn();
    const unsubscribe = subscribeTeaching(listener);

    updateTeachingState(CENTER_A, (state) => ({
      ...state,
      lessonPlans: { [lessonPlanKey("class-1", 0)]: makePlan("draft") },
    }));

    expect(listener).toHaveBeenCalledTimes(1);
    expect(getTeachingSnapshot(CENTER_A)).not.toBe(before);

    unsubscribe();
    updateTeachingState(CENTER_A, (state) => state);
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it("counts only pending lesson plans", () => {
    updateTeachingState(CENTER_A, (state) => ({
      ...state,
      lessonPlans: {
        [lessonPlanKey("class-1", 0)]: makePlan("pending"),
        [lessonPlanKey("class-1", 1)]: makePlan("approved"),
        [lessonPlanKey("class-2", 0)]: makePlan("pending"),
        [lessonPlanKey("class-2", 1)]: makePlan("redo"),
      },
    }));
    expect(countPendingPlans(getTeachingSnapshot(CENTER_A))).toBe(2);
    expect(countPendingPlans(getTeachingSnapshot(CENTER_B))).toBe(0);
  });

  it("exposes the per-session cost constant the prototype pins at 300.000đ", () => {
    expect(SESSION_COST_VND).toBe(300_000);
  });
});
