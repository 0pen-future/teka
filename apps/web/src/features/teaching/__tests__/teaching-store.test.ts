import { describe, expect, it } from "vitest";

import {
  lessonPlanKey,
  personalNoteKey,
  SESSION_COST_VND,
  transitionLessonPlanStatus,
  type LessonPlanAction,
  type LessonPlanStatus,
} from "../lib/teaching-store";

describe("teaching domain helpers", () => {
  it("builds composite keys the way the API store and caches expect", () => {
    expect(lessonPlanKey("class-1", 2)).toBe("class-1#2");
    expect(personalNoteKey("session-05", "student-9")).toBe("session-05#student-9");
  });

  it("exposes the per-session cost constant the prototype pins at 300.000đ", () => {
    expect(SESSION_COST_VND).toBe(300_000);
  });
});

describe("transitionLessonPlanStatus", () => {
  const legal: [LessonPlanStatus, LessonPlanAction, LessonPlanStatus][] = [
    ["none", "save", "draft"],
    ["draft", "save", "draft"],
    ["draft", "submit", "pending"],
    ["pending", "approve", "approved"],
    ["pending", "requestRedo", "redo"],
    ["redo", "save", "redo"],
    ["redo", "submit", "pending"],
    ["redo", "reopen", "pending"],
    ["approved", "reopen", "pending"],
  ];

  it.each(legal)("allows %s + %s → %s", (status, action, expected) => {
    expect(transitionLessonPlanStatus(status, action)).toBe(expected);
  });

  it("rejects every other status/action combination", () => {
    const statuses: LessonPlanStatus[] = ["none", "draft", "pending", "approved", "redo"];
    const actions: LessonPlanAction[] = ["save", "submit", "approve", "requestRedo", "reopen"];
    const legalSet = new Set(legal.map(([status, action]) => `${status}:${action}`));
    let illegalCount = 0;
    for (const status of statuses) {
      for (const action of actions) {
        if (!legalSet.has(`${status}:${action}`)) {
          expect(transitionLessonPlanStatus(status, action)).toBeNull();
          illegalCount += 1;
        }
      }
    }
    expect(illegalCount).toBe(16);
  });
});
