import { describe, expect, it } from "vitest";

import {
  importErrorsPayloadSchema,
  importReportSchema,
  importRowErrorSchema,
} from "../schemas/import-schemas";

const entity = { created: 0, reused: 0 };

const report = {
  committed: false,
  classes: { created: 2, reused: 1 },
  schedules: entity,
  contacts: entity,
  students: entity,
  enrollments: entity,
};

describe("importReportSchema", () => {
  it("parses a report with all five entities", () => {
    expect(importReportSchema.parse(report)).toEqual(report);
  });

  it("rejects a report missing an entity", () => {
    const { committed, classes, schedules, contacts, students } = report;

    expect(
      importReportSchema.safeParse({ committed, classes, schedules, contacts, students }).success,
    ).toBe(false);
  });
});

describe("importRowErrorSchema", () => {
  it("accepts a defect with no column — a whole-row rule points at no cell", () => {
    const parsed = importRowErrorSchema.parse({
      sheet: "HocSinh",
      line: 4,
      code: "ENROLLMENT_ENDED",
      message: "học sinh này đã nghỉ lớp từ 30/11/2025",
    });

    expect(parsed.column).toBeUndefined();
  });
});

describe("importErrorsPayloadSchema", () => {
  it("parses a list with no truncation marker", () => {
    const parsed = importErrorsPayloadSchema.parse({
      errors: [{ sheet: "Lop", line: 3, column: "Tên lớp", code: "TOO_LONG", message: "quá dài" }],
    });

    expect(parsed.errors).toHaveLength(1);
    expect(parsed.truncated).toBeUndefined();
  });

  it("keeps the omitted-defect count when the server capped the list", () => {
    expect(importErrorsPayloadSchema.parse({ errors: [], truncated: 42 }).truncated).toBe(42);
  });

  it("rejects a payload that is not the errors envelope", () => {
    expect(importErrorsPayloadSchema.safeParse({ message: "sai" }).success).toBe(false);
  });
});
