import { describe, expect, it } from "vitest";

import { phaseLabel, phaseVariant, shiftOptions, weekdayOptions } from "../lib/class-labels";
import { classPhases } from "../schemas/roster-schemas";

describe("class labels", () => {
  it("names every phase the API can report", () => {
    expect(phaseLabel).toEqual({
      upcoming: "Sắp khai giảng",
      running: "Đang học",
      ended: "Đã kết thúc",
      archived: "Lưu trữ",
    });
    for (const phase of classPhases) {
      expect(phaseVariant[phase]).toBeTruthy();
    }
  });

  it("lists weekdays Monday-first after an all-days option, using the schedule weekday numbers", () => {
    expect(weekdayOptions[0]).toEqual({ value: "", label: "Tất cả các ngày" });
    expect(weekdayOptions.map((option) => option.value)).toEqual([
      "",
      "1",
      "2",
      "3",
      "4",
      "5",
      "6",
      "0",
    ]);
    expect(weekdayOptions[1]!.label).toBe("Thứ 2");
    expect(weekdayOptions[7]!.label).toBe("Chủ nhật");
  });

  it("lists the three shifts after an all-shifts option", () => {
    expect(shiftOptions).toEqual([
      { value: "", label: "Tất cả ca" },
      { value: "morning", label: "Sáng" },
      { value: "afternoon", label: "Chiều" },
      { value: "evening", label: "Tối" },
    ]);
  });
});
