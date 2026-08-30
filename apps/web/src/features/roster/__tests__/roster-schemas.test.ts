import { describe, expect, it } from "vitest";

import {
  classDialogInputSchema,
  classSchema,
  classSettingsInputSchema,
  toClassCreateInput,
  type ClassDialogInput,
} from "../schemas/roster-schemas";

function dialogInput(slots: ClassDialogInput["slots"]): ClassDialogInput {
  return {
    name: "Toán 9C",
    start_date: "2026-08-05",
    end_date: "",
    default_unit_price: 150_000,
    slots,
    duration_min: 90,
  };
}

describe("toClassCreateInput", () => {
  it("flattens slots into one schedule row per (weekday, time) pair", () => {
    const input = toClassCreateInput(
      dialogInput([
        { start_time: "18:00", days: [1, 3] },
        { start_time: "20:00", days: [6] },
      ]),
    );
    expect(input.schedules).toEqual([
      { weekday: 1, start_time: "18:00", duration_min: 90, effective_from: "2026-08-05" },
      { weekday: 3, start_time: "18:00", duration_min: 90, effective_from: "2026-08-05" },
      { weekday: 6, start_time: "20:00", duration_min: 90, effective_from: "2026-08-05" },
    ]);
    expect(input).not.toHaveProperty("slots");
    expect(input).not.toHaveProperty("duration_min");
  });

  it("never emits the same (weekday, time) pair twice", () => {
    const input = toClassCreateInput(
      dialogInput([
        { start_time: "18:00", days: [1, 1] },
        { start_time: "18:00", days: [1] },
      ]),
    );
    expect(input.schedules).toEqual([
      { weekday: 1, start_time: "18:00", duration_min: 90, effective_from: "2026-08-05" },
    ]);
  });
});

describe("classSchema", () => {
  function classResponse() {
    return {
      id: "70000000-0000-4000-8000-000000000001",
      name: "Toán 6A",
      teacher_id: "73000000-0000-4000-8000-000000000001",
      start_date: "2026-01-05",
      end_date: null,
      default_unit_price: 150_000,
      status: "active",
      schedules: [],
      created_at: "2026-01-01T08:00:00Z",
    };
  }

  it("defaults my_staff_roles to [] when the response omits it", () => {
    const result = classSchema.safeParse(classResponse());
    expect(result.success).toBe(true);
    expect(result.data?.my_staff_roles).toEqual([]);
  });

  it("keeps the response's my_staff_roles when present", () => {
    const result = classSchema.safeParse({ ...classResponse(), my_staff_roles: ["giao_vien"] });
    expect(result.success).toBe(true);
    expect(result.data?.my_staff_roles).toEqual(["giao_vien"]);
  });
});

describe("khung-giờ slot validation", () => {
  // The API would accept two rows on one weekday, but the session generator
  // materializes at most one session per class per date — the second row
  // would silently never run. Both forms must reject it up front.
  it("rejects a weekday appearing in two slots, flagging the later slot", () => {
    const result = classDialogInputSchema.safeParse(
      dialogInput([
        { start_time: "18:00", days: [1, 3] },
        { start_time: "20:00", days: [3] },
      ]),
    );
    expect(result.success).toBe(false);
    const issue = result.error?.issues.find((i) => i.code === "custom");
    expect(issue?.path).toEqual(["slots", 1, "days"]);
    expect(issue?.message).toContain("mỗi ngày chỉ một khung giờ");
  });

  it("applies the same duplicate-weekday rule to the settings form", () => {
    const result = classSettingsInputSchema.safeParse({
      name: "Toán 9C",
      default_unit_price: 150_000,
      slots: [
        { start_time: "18:00", days: [2] },
        { start_time: "19:30", days: [2] },
      ],
    });
    expect(result.success).toBe(false);
    expect(result.error?.issues.some((i) => i.code === "custom")).toBe(true);
  });

  it("accepts distinct weekdays across two slots", () => {
    const result = classDialogInputSchema.safeParse(
      dialogInput([
        { start_time: "18:00", days: [1, 3] },
        { start_time: "20:00", days: [6] },
      ]),
    );
    expect(result.success).toBe(true);
  });
});
