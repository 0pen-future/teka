import { describe, expect, it } from "vitest";

import {
  formatScheduleLabel,
  formatScheduleLines,
  formatScheduleSummary,
} from "../lib/roster-format";
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

describe("formatScheduleLabel", () => {
  it("spells out a single weekday with its day part", () => {
    expect(
      formatScheduleLabel([schedule({ id: "a", weekday: 2, start_time: "18:00" })], TODAY),
    ).toBe("Tối Thứ Ba");
    expect(
      formatScheduleLabel([schedule({ id: "a", weekday: 0, start_time: "08:30:00" })], TODAY),
    ).toBe("Sáng Chủ Nhật");
    expect(
      formatScheduleLabel([schedule({ id: "a", weekday: 6, start_time: "14:00" })], TODAY),
    ).toBe("Chiều Thứ Bảy");
  });

  it("collapses several weekdays of one khung giờ to short names, Monday-first", () => {
    const label = formatScheduleLabel(
      [
        schedule({ id: "a", weekday: 3, start_time: "18:00" }),
        schedule({ id: "b", weekday: 1, start_time: "18:00" }),
        schedule({ id: "c", weekday: 0, start_time: "18:00" }),
      ],
      TODAY,
    );
    expect(label).toBe("Tối T2-T4-CN");
  });

  it("joins several khung giờ with a comma", () => {
    const label = formatScheduleLabel(
      [
        schedule({ id: "a", weekday: 2, start_time: "19:00" }),
        schedule({ id: "b", weekday: 4, start_time: "19:00" }),
        schedule({ id: "c", weekday: 6, start_time: "09:00" }),
      ],
      TODAY,
    );
    expect(label).toBe("Sáng Thứ Bảy, Tối T3-T5");
  });

  it("drops the day part when the start time does not parse", () => {
    expect(
      formatScheduleLabel([schedule({ id: "a", weekday: 2, start_time: "soon" })], TODAY),
    ).toBe("Thứ Ba");
  });

  it("ignores closed rows and returns an empty string with no timetable", () => {
    expect(
      formatScheduleLabel(
        [
          schedule({ id: "a", weekday: 1, start_time: "18:00" }),
          schedule({ id: "b", weekday: 4, start_time: "18:00", effective_to: "2026-07-01" }),
        ],
        TODAY,
      ),
    ).toBe("Tối Thứ Hai");
    expect(formatScheduleLabel([], TODAY)).toBe("");
  });
});

describe("formatScheduleLines", () => {
  it("prints one line per active session, Monday-first, with its end time and shift", () => {
    expect(
      formatScheduleLines(
        [
          schedule({ id: "sun", weekday: 0, start_time: "09:30:00", duration_min: 90 }),
          schedule({ id: "tue-late", weekday: 2, start_time: "17:30", duration_min: 90 }),
          schedule({ id: "tue-early", weekday: 2, start_time: "17:15", duration_min: 90 }),
          schedule({ id: "closed", weekday: 1, effective_to: "2026-08-04" }),
          schedule({ id: "late", weekday: 6, start_time: "23:30", duration_min: 60 }),
        ],
        TODAY,
      ),
    ).toEqual([
      { key: "tue-early", text: "Thứ Ba, 17:15 - 18:45", shift: "afternoon" },
      { key: "tue-late", text: "Thứ Ba, 17:30 - 19:00", shift: "evening" },
      { key: "late", text: "Thứ Bảy, 23:30 - 00:30", shift: "evening" },
      { key: "sun", text: "Chủ Nhật, 09:30 - 11:00", shift: "morning" },
    ]);
    expect(formatScheduleLines([], TODAY)).toEqual([]);
  });
});
