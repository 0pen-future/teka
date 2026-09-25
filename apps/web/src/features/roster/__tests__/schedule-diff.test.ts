import { describe, expect, it } from "vitest";

import { activeScheduleRows, deriveScheduleSlots, diffScheduleRows } from "../lib/schedule-diff";
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

describe("deriveScheduleSlots", () => {
  it("groups active rows into one slot per start time, earliest first", () => {
    const slots = deriveScheduleSlots(
      [
        schedule({ id: "a", weekday: 1, start_time: "19:00" }),
        schedule({ id: "b", weekday: 3, start_time: "19:00" }),
        schedule({ id: "c", weekday: 5, start_time: "18:00" }),
      ],
      TODAY,
    );
    expect(slots).toEqual([
      { start_time: "18:00", days: [5] },
      { start_time: "19:00", days: [1, 3] },
    ]);
  });

  it("ignores rows already closed before today and normalizes HH:MM:SS", () => {
    const slots = deriveScheduleSlots(
      [
        schedule({ id: "a", weekday: 1, start_time: "19:00:00" }),
        schedule({ id: "b", weekday: 2, effective_to: "2026-07-01" }),
      ],
      TODAY,
    );
    expect(slots).toEqual([{ start_time: "19:00", days: [1] }]);
  });

  it("lists a weekday only once per slot even with duplicate rows", () => {
    const slots = deriveScheduleSlots(
      [
        schedule({ id: "a", weekday: 1, start_time: "19:00" }),
        schedule({ id: "b", weekday: 1, start_time: "19:00" }),
      ],
      TODAY,
    );
    expect(slots).toEqual([{ start_time: "19:00", days: [1] }]);
  });
});

const row = (weekday: number, start_time = "19:00", duration_min: number | null = 90) => ({
  weekday,
  start_time,
  duration_min,
});

describe("activeScheduleRows", () => {
  it("lists active rows Monday first with their own durations", () => {
    const rows = activeScheduleRows(
      [
        schedule({ id: "a", weekday: 0, start_time: "09:00:00", duration_min: 120 }),
        schedule({ id: "b", weekday: 3 }),
        schedule({ id: "c", weekday: 1, effective_to: "2026-07-01" }),
      ],
      TODAY,
    );
    expect(rows).toEqual([row(3), row(0, "09:00", 120)]);
  });
});

describe("diffScheduleRows", () => {
  it("returns an empty diff when the rows match the active schedules", () => {
    const rows = [schedule({ id: "a", weekday: 1 }), schedule({ id: "b", weekday: 3 })];
    expect(diffScheduleRows(rows, [row(1), row(3)], TODAY)).toEqual({
      toAdd: [],
      toClose: [],
      toDelete: [],
    });
  });

  it("closes a removed row and adds a new one", () => {
    const rows = [schedule({ id: "a", weekday: 1 }), schedule({ id: "b", weekday: 3 })];
    const diff = diffScheduleRows(rows, [row(3), row(6)], TODAY);
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

  it("replaces a row whose duration alone changed", () => {
    const diff = diffScheduleRows(
      [schedule({ id: "a", weekday: 1 })],
      [row(1, "19:00", 120)],
      TODAY,
    );
    expect(diff.toClose.map((close) => close.id)).toEqual(["a"]);
    expect(diff.toAdd).toEqual([
      { weekday: 1, start_time: "19:00", duration_min: 120, effective_from: TODAY },
    ]);
  });

  it("deletes outright a replaced row that has not taken effect yet", () => {
    const rows = [schedule({ id: "a", weekday: 1, effective_from: "2026-08-09" })];
    const diff = diffScheduleRows(rows, [row(1, "20:00")], TODAY);
    expect(diff.toClose).toEqual([]);
    expect(diff.toDelete).toEqual(["a"]);
    expect(diff.toAdd).toEqual([
      { weekday: 1, start_time: "20:00", duration_min: 90, effective_from: TODAY },
    ]);
  });

  it("leaves rows closed before today alone and still re-adds their weekday", () => {
    const rows = [schedule({ id: "a", weekday: 1, effective_to: "2026-07-01" })];
    const diff = diffScheduleRows(rows, [row(1)], TODAY);
    expect(diff.toClose).toEqual([]);
    expect(diff.toDelete).toEqual([]);
    expect(diff.toAdd).toEqual([
      { weekday: 1, start_time: "19:00", duration_min: 90, effective_from: TODAY },
    ]);
  });

  it("retires the whole timetable when no rows are wanted", () => {
    const rows = [
      schedule({ id: "a", weekday: 1 }),
      schedule({ id: "b", weekday: 3, effective_from: "2026-08-10" }),
    ];
    const diff = diffScheduleRows(rows, [], TODAY);
    expect(diff.toClose.map((close) => close.id)).toEqual(["a"]);
    expect(diff.toDelete).toEqual(["b"]);
    expect(diff.toAdd).toEqual([]);
  });

  it("collapses duplicate rows on the same weekday, time and duration down to one", () => {
    const rows = [schedule({ id: "a", weekday: 1 }), schedule({ id: "b", weekday: 1 })];
    const diff = diffScheduleRows(rows, [row(1)], TODAY);
    expect(diff.toClose.map((close) => close.id)).toEqual(["b"]);
    expect(diff.toAdd).toEqual([]);
  });

  it("never double-adds a row listed twice", () => {
    const diff = diffScheduleRows([], [row(4), row(4)], TODAY);
    expect(diff.toAdd).toEqual([
      { weekday: 4, start_time: "19:00", duration_min: 90, effective_from: TODAY },
    ]);
  });
});
