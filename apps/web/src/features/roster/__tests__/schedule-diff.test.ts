import { describe, expect, it } from "vitest";

import { deriveScheduleForm, diffSchedules } from "../lib/schedule-diff";
import type { Schedule } from "../schemas/roster-schemas";

const TODAY = "2026-08-05";
const YESTERDAY = "2026-08-04";

function schedule(overrides: Partial<Schedule>): Schedule {
  return {
    id: "schedule-1",
    weekday: 1,
    start_time: "19:00",
    duration_min: 90,
    effective_from: "2026-01-05",
    effective_to: null,
    ...overrides,
  };
}

describe("deriveScheduleForm", () => {
  it("collects unique weekdays and the most common start time", () => {
    const form = deriveScheduleForm(
      [
        schedule({ id: "a", weekday: 1, start_time: "19:00" }),
        schedule({ id: "b", weekday: 3, start_time: "19:00" }),
        schedule({ id: "c", weekday: 5, start_time: "18:00" }),
      ],
      TODAY,
    );
    expect(form.days.sort()).toEqual([1, 3, 5]);
    expect(form.start_time).toBe("19:00");
  });

  it("ignores rows already closed before today and normalizes HH:MM:SS", () => {
    const form = deriveScheduleForm(
      [
        schedule({ id: "a", weekday: 1, start_time: "19:00:00" }),
        schedule({ id: "b", weekday: 2, effective_to: "2026-07-01" }),
      ],
      TODAY,
    );
    expect(form.days).toEqual([1]);
    expect(form.start_time).toBe("19:00");
  });
});

describe("diffSchedules", () => {
  it("returns an empty diff when days and time are unchanged", () => {
    const rows = [schedule({ id: "a", weekday: 1 }), schedule({ id: "b", weekday: 3 })];
    expect(diffSchedules(rows, [1, 3], "19:00", TODAY)).toEqual({
      toAdd: [],
      toClose: [],
      toDelete: [],
    });
  });

  it("closes rows for deselected weekdays and adds rows for new ones", () => {
    const rows = [schedule({ id: "a", weekday: 1 }), schedule({ id: "b", weekday: 3 })];
    const diff = diffSchedules(rows, [3, 6], "19:00", TODAY);
    expect(diff.toClose).toEqual([
      {
        id: "a",
        input: {
          weekday: 1,
          start_time: "19:00",
          duration_min: 90,
          effective_from: "2026-01-05",
          effective_to: YESTERDAY,
        },
      },
    ]);
    expect(diff.toDelete).toEqual([]);
    expect(diff.toAdd).toEqual([
      { weekday: 6, start_time: "19:00", duration_min: 90, effective_from: TODAY },
    ]);
  });

  it("replaces every kept weekday's row on a time change, preserving its duration", () => {
    const rows = [
      schedule({ id: "a", weekday: 1, duration_min: 120 }),
      schedule({ id: "b", weekday: 3, duration_min: 90 }),
    ];
    const diff = diffSchedules(rows, [1, 3], "20:30", TODAY);
    expect(diff.toClose.map((close) => close.id).sort()).toEqual(["a", "b"]);
    expect(diff.toClose.every((close) => close.input.effective_to === YESTERDAY)).toBe(true);
    expect(diff.toAdd).toEqual([
      { weekday: 1, start_time: "20:30", duration_min: 120, effective_from: TODAY },
      { weekday: 3, start_time: "20:30", duration_min: 90, effective_from: TODAY },
    ]);
  });

  it("deletes outright a replaced row that has not taken effect yet", () => {
    const rows = [schedule({ id: "a", weekday: 1, effective_from: "2026-08-09" })];
    const diff = diffSchedules(rows, [1], "20:00", TODAY);
    expect(diff.toClose).toEqual([]);
    expect(diff.toDelete).toEqual(["a"]);
    expect(diff.toAdd).toEqual([
      { weekday: 1, start_time: "20:00", duration_min: 90, effective_from: TODAY },
    ]);
  });

  it("leaves rows closed before today alone and still re-adds their weekday", () => {
    const rows = [schedule({ id: "a", weekday: 1, effective_to: "2026-07-01" })];
    const diff = diffSchedules(rows, [1], "19:00", TODAY);
    expect(diff.toClose).toEqual([]);
    expect(diff.toDelete).toEqual([]);
    expect(diff.toAdd).toEqual([
      { weekday: 1, start_time: "19:00", duration_min: 90, effective_from: TODAY },
    ]);
  });

  it("collapses duplicate rows on the same weekday down to one", () => {
    const rows = [
      schedule({ id: "a", weekday: 1, start_time: "19:00" }),
      schedule({ id: "b", weekday: 1, start_time: "19:00" }),
    ];
    const diff = diffSchedules(rows, [1], "19:00", TODAY);
    expect(diff.toClose.map((close) => close.id)).toEqual(["b"]);
    expect(diff.toAdd).toEqual([]);
  });

  it("never double-adds a weekday listed twice in the selection", () => {
    const diff = diffSchedules([], [4, 4], "19:00", TODAY);
    expect(diff.toAdd).toEqual([
      { weekday: 4, start_time: "19:00", duration_min: 90, effective_from: TODAY },
    ]);
  });
});
