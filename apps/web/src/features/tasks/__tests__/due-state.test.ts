import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { localIsoDate, quickDueOptions } from "../lib/due-state";

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
