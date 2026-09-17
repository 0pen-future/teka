import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { dueState, localIsoDate, quickDueOptions } from "../lib/due-state";

// The suite runs with `TZ=Asia/Ho_Chi_Minh` (see vitest.config.ts), so `Date`'s
// local getters read VN wall-clock time while `toISOString()`/`getUTCDate()`
// stay on UTC — exactly the discrepancy these helpers must avoid.

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("localIsoDate", () => {
  it("reads the local (VN) calendar day, not UTC, near a UTC midnight rollover", () => {
    // 23:30 UTC is already 06:30 the next VN day (UTC+7): the two calendars
    // disagree on what "today" is at this instant.
    vi.setSystemTime(new Date("2026-09-17T23:30:00Z"));
    expect(localIsoDate(new Date())).toBe("2026-09-18");
    expect(new Date().toISOString().slice(0, 10)).toBe("2026-09-17");
  });

  it("pads single-digit month and day", () => {
    vi.setSystemTime(new Date("2026-01-05T03:00:00+07:00"));
    expect(localIsoDate(new Date())).toBe("2026-01-05");
  });
});

describe("quickDueOptions", () => {
  it("sets Hôm nay/Ngày mai to the local today/tomorrow", () => {
    vi.setSystemTime(new Date("2026-09-17T10:00:00+07:00")); // Thursday
    const options = quickDueOptions(new Date());
    expect(options).toEqual([
      { label: "Hôm nay", value: "2026-09-17" },
      { label: "Ngày mai", value: "2026-09-18" },
      { label: "Thứ 2 tới", value: "2026-09-21" },
      { label: "Bỏ hạn", value: "" },
    ]);
  });

  it("rolls Thứ 2 tới to next week when today is already Monday", () => {
    vi.setSystemTime(new Date("2026-09-14T10:00:00+07:00")); // Monday
    const options = quickDueOptions(new Date());
    expect(options.find((option) => option.label === "Thứ 2 tới")?.value).toBe("2026-09-21");
  });

  it("rolls Thứ 2 tới to the very next day when today is Sunday", () => {
    vi.setSystemTime(new Date("2026-09-20T10:00:00+07:00")); // Sunday
    const options = quickDueOptions(new Date());
    expect(options.find((option) => option.label === "Thứ 2 tới")?.value).toBe("2026-09-21");
  });

  it("Bỏ hạn always clears the value", () => {
    vi.setSystemTime(new Date("2026-09-17T10:00:00+07:00"));
    expect(quickDueOptions(new Date()).find((option) => option.label === "Bỏ hạn")?.value).toBe("");
  });
});

describe("dueState", () => {
  it("is not overdue for a task due today, even at 18:30 VN (23:30 UTC, the next UTC day)", () => {
    vi.setSystemTime(new Date("2026-09-17T18:30:00+07:00"));
    const state = dueState({ dueOn: "2026-09-17", completedAt: null });
    expect(state).toMatchObject({ kind: "today", label: "Hôm nay", variant: "warning" });
  });

  it("reports days overdue for a task due yesterday", () => {
    vi.setSystemTime(new Date("2026-09-17T10:00:00+07:00"));
    const state = dueState({ dueOn: "2026-09-16", completedAt: null });
    expect(state).toMatchObject({
      kind: "overdue",
      days: 1,
      label: "Quá hạn 1 ngày",
      variant: "danger",
    });
  });

  it("labels a task due tomorrow", () => {
    vi.setSystemTime(new Date("2026-09-17T10:00:00+07:00"));
    const state = dueState({ dueOn: "2026-09-18", completedAt: null });
    expect(state).toMatchObject({ kind: "tomorrow", label: "Ngày mai", variant: "neutral" });
  });

  it("labels a task due further out with its dd/MM date", () => {
    vi.setSystemTime(new Date("2026-09-17T10:00:00+07:00"));
    const state = dueState({ dueOn: "2026-09-25", completedAt: null });
    expect(state).toMatchObject({ kind: "upcoming", label: "Hạn 25/09", variant: "neutral" });
  });

  it("resolves to none when the task has no due date", () => {
    vi.setSystemTime(new Date("2026-09-17T10:00:00+07:00"));
    expect(dueState({ dueOn: null, completedAt: null }).kind).toBe("none");
  });

  it("resolves to none once the task is completed, even if it was overdue", () => {
    vi.setSystemTime(new Date("2026-09-17T10:00:00+07:00"));
    const state = dueState({ dueOn: "2026-09-01", completedAt: "2026-09-05T08:00:00Z" });
    expect(state.kind).toBe("none");
  });
});
