import { describe, expect, it } from "vitest";

import {
  classDialogInputSchema,
  classSchema,
  classWizardInputSchema,
  toClassCreateInput,
  toClassUpdateInput,
  type Class,
  type ClassDialogInput,
  type ClassWizardInput,
} from "../schemas/roster-schemas";

function dialogInput(slots: ClassDialogInput["slots"]): ClassDialogInput {
  return {
    name: "Toán 9C",
    start_date: "2026-08-05",
    end_date: "",
    default_unit_price: 150_000,
    slots,
    duration_min: 90,
    course_id: "",
  };
}

function wizardInput(overrides: Partial<ClassWizardInput>): ClassWizardInput {
  return {
    name: "Toán 9C",
    course_id: "90000000-0000-4000-8000-000000000001",
    code: "TOAN9C",
    default_unit_price: 150_000,
    tags: [],
    parent_class_id: "",
    next_class_id: "",
    study_mode: "scheduled",
    start_date: "2026-08-05",
    end_date: "",
    slots: [{ weekday: 2, start_time: "18:00", duration_min: 90 }],
    room: "",
    teacher_id: "",
    note: "",
    ...overrides,
  };
}

describe("toClassCreateInput", () => {
  it("omits course_id when no course was picked and passes it through otherwise", () => {
    const blank = toClassCreateInput(dialogInput([{ start_time: "18:00", days: [1] }]));
    expect("course_id" in blank).toBe(false);
    const attached = toClassCreateInput({
      ...dialogInput([{ start_time: "18:00", days: [1] }]),
      course_id: "90000000-0000-4000-8000-000000000001",
    });
    expect(attached.course_id).toBe("90000000-0000-4000-8000-000000000001");
  });

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

  it("rejects a repeated weekday in the class wizard", () => {
    const result = classWizardInputSchema.safeParse(
      wizardInput({
        slots: [
          { weekday: 2, start_time: "18:00", duration_min: 90 },
          { weekday: 2, start_time: "19:30", duration_min: 90 },
        ],
      }),
    );
    expect(result.success).toBe(false);
    expect(result.error?.issues[0]?.path).toEqual(["slots", 1, "weekday"]);
  });

  it("requires time and duration on every wizard row of a scheduled class", () => {
    const result = classWizardInputSchema.safeParse(
      wizardInput({ slots: [{ weekday: 2, start_time: "", duration_min: null }] }),
    );
    expect(result.success).toBe(false);
    expect(result.error?.issues.map((issue) => issue.message)).toEqual([
      "Mỗi lịch học cần giờ bắt đầu và thời lượng",
      "Mỗi lịch học cần giờ bắt đầu và thời lượng",
    ]);
  });

  it("lets a self-paced class save without a timetable or start date", () => {
    const result = classWizardInputSchema.safeParse(
      wizardInput({ study_mode: "self_paced", start_date: "", slots: [] }),
    );
    expect(result.success).toBe(true);
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

describe("classSchema catalog fields", () => {
  it("defaults the catalog fields for a response that predates them", () => {
    const parsed = classSchema.parse({
      id: "c1",
      name: "Toán 9C",
      teacher_id: "t1",
      start_date: "2026-08-05",
      end_date: null,
      default_unit_price: 150_000,
      status: "active",
      schedules: [],
      created_at: "2026-08-01T00:00:00Z",
    });
    expect(parsed.code).toBe("");
    expect(parsed.tags).toEqual([]);
    expect(parsed.recruiting).toBe(false);
    expect(parsed.note).toBeNull();
    expect(parsed.phase).toBe("running");
  });
});

describe("toClassUpdateInput", () => {
  const klass: Class = classSchema.parse({
    id: "c1",
    name: "Toán 9C",
    teacher_id: "t1",
    start_date: "2026-08-05",
    end_date: "2026-12-20",
    default_unit_price: 150_000,
    status: "active",
    schedules: [],
    created_at: "2026-08-01T00:00:00Z",
    code: "TOAN9C",
    tags: ["Toán", "Khối 9"],
    recruiting: false,
    note: "Phòng 201",
    phase: "running",
  });

  it("copies the required base fields and adds only the catalog fields that changed", () => {
    expect(toClassUpdateInput(klass, { recruiting: true })).toEqual({
      name: "Toán 9C",
      start_date: "2026-08-05",
      end_date: "2026-12-20",
      default_unit_price: 150_000,
      recruiting: true,
    });
  });

  it("omits every catalog field when nothing differs from the class", () => {
    const body = toClassUpdateInput(klass, {
      recruiting: false,
      tags: ["Toán", "Khối 9"],
      note: "Phòng 201",
    });
    expect(body).not.toHaveProperty("recruiting");
    expect(body).not.toHaveProperty("tags");
    expect(body).not.toHaveProperty("note");
    expect(body).not.toHaveProperty("code");
  });

  it("sends an empty note to clear it and an empty end_date for an open-ended class", () => {
    const openEnded = { ...klass, end_date: null };
    expect(toClassUpdateInput(openEnded, { note: "", tags: ["Toán"] })).toEqual({
      name: "Toán 9C",
      start_date: "2026-08-05",
      end_date: "",
      default_unit_price: 150_000,
      note: "",
      tags: ["Toán"],
    });
  });
});
