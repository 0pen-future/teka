import { describe, expect, it } from "vitest";

import { deriveScheduleSlots, diffSchedules } from "../lib/schedule-diff";
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

describe("diffSchedules", () => {
  it("returns an empty diff when the slots match the active rows", () => {
    const rows = [schedule({ id: "a", weekday: 1 }), schedule({ id: "b", weekday: 3 })];
    expect(diffSchedules(rows, [{ start_time: "19:00", days: [1, 3] }], TODAY)).toEqual({
      toAdd: [],
      toClose: [],
      toDelete: [],
    });
  });

  it("closes rows for deselected weekdays and adds rows for new ones", () => {
    const rows = [schedule({ id: "a", weekday: 1 }), schedule({ id: "b", weekday: 3 })];
    const diff = diffSchedules(rows, [{ start_time: "19:00", days: [3, 6] }], TODAY);
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
    const diff = diffSchedules(rows, [{ start_time: "20:30", days: [1, 3] }], TODAY);
    expect(diff.toClose.map((close) => close.id).sort()).toEqual(["a", "b"]);
    expect(diff.toClose.every((close) => close.input.effective_to === YESTERDAY)).toBe(true);
    expect(diff.toAdd).toEqual([
      { weekday: 1, start_time: "20:30", duration_min: 120, effective_from: TODAY },
      { weekday: 3, start_time: "20:30", duration_min: 90, effective_from: TODAY },
    ]);
  });

  // The forms reject a weekday spanning two slots (`classSlotsField`), but the
  // diff must still leave such legacy rows alone when nothing changed.
  it("keeps a weekday's rows in two slots with different times apart", () => {
    const rows = [
      schedule({ id: "a", weekday: 1, start_time: "18:00" }),
      schedule({ id: "b", weekday: 1, start_time: "20:00" }),
    ];
    const diff = diffSchedules(
      rows,
      [
        { start_time: "18:00", days: [1] },
        { start_time: "20:00", days: [1] },
      ],
      TODAY,
    );
    expect(diff).toEqual({ toAdd: [], toClose: [], toDelete: [] });
  });

  it("deletes outright a replaced row that has not taken effect yet", () => {
    const rows = [schedule({ id: "a", weekday: 1, effective_from: "2026-08-09" })];
    const diff = diffSchedules(rows, [{ start_time: "20:00", days: [1] }], TODAY);
    expect(diff.toClose).toEqual([]);
    expect(diff.toDelete).toEqual(["a"]);
    expect(diff.toAdd).toEqual([
      { weekday: 1, start_time: "20:00", duration_min: 90, effective_from: TODAY },
    ]);
  });

  it("leaves rows closed before today alone and still re-adds their weekday", () => {
    const rows = [schedule({ id: "a", weekday: 1, effective_to: "2026-07-01" })];
    const diff = diffSchedules(rows, [{ start_time: "19:00", days: [1] }], TODAY);
    expect(diff.toClose).toEqual([]);
    expect(diff.toDelete).toEqual([]);
    expect(diff.toAdd).toEqual([
      { weekday: 1, start_time: "19:00", duration_min: 90, effective_from: TODAY },
    ]);
  });

  it("collapses duplicate rows on the same weekday and time down to one", () => {
    const rows = [
      schedule({ id: "a", weekday: 1, start_time: "19:00" }),
      schedule({ id: "b", weekday: 1, start_time: "19:00" }),
    ];
    const diff = diffSchedules(rows, [{ start_time: "19:00", days: [1] }], TODAY);
    expect(diff.toClose.map((close) => close.id)).toEqual(["b"]);
    expect(diff.toAdd).toEqual([]);
  });

  it("never double-adds a pair listed twice across the slots", () => {
    const diff = diffSchedules(
      [],
      [
        { start_time: "19:00", days: [4, 4] },
        { start_time: "19:00", days: [4] },
      ],
      TODAY,
    );
    expect(diff.toAdd).toEqual([
      { weekday: 4, start_time: "19:00", duration_min: 90, effective_from: TODAY },
    ]);
  });
});
