import { describe, expect, it } from "vitest";

import { formatScheduleSummary } from "../lib/roster-format";
import type { Schedule } from "../schemas/roster-schemas";

const TODAY = "2026-08-05";

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

describe("formatScheduleSummary", () => {
  it("renders one segment per start time with Monday-first weekdays", () => {
    const label = formatScheduleSummary(
      [
        schedule({ id: "a", weekday: 3, start_time: "18:00" }),
        schedule({ id: "b", weekday: 1, start_time: "18:00" }),
        schedule({ id: "c", weekday: 6, start_time: "20:00" }),
      ],
      TODAY,
    );
    expect(label).toBe("T2 · T4 — 18:00, T7 — 20:00");
  });

  it("normalizes HH:MM:SS server times and orders Chủ nhật last", () => {
    const label = formatScheduleSummary(
      [
        schedule({ id: "a", weekday: 0, start_time: "18:00:00" }),
        schedule({ id: "b", weekday: 2, start_time: "18:00:00" }),
      ],
      TODAY,
    );
    expect(label).toBe("T3 · CN — 18:00");
  });

  it("excludes rows already closed before today", () => {
    const label = formatScheduleSummary(
      [
        schedule({ id: "a", weekday: 1 }),
        schedule({ id: "b", weekday: 4, effective_to: "2026-07-01" }),
      ],
      TODAY,
    );
    expect(label).toBe("T2 — 19:00");
  });

  it("returns an empty string for a class with no active timetable", () => {
    expect(formatScheduleSummary([], TODAY)).toBe("");
  });
});
