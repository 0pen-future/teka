import { describe, expect, it } from "vitest";

import { filterStudentRows } from "../lib/student-search";

const rows = [{ name: "Nguyễn Văn An" }, { name: "Trần Thị Bình" }, { name: "Lê Đức Anh" }];

describe("filterStudentRows", () => {
  it("returns the same array for a blank query", () => {
    expect(filterStudentRows(rows, "")).toBe(rows);
    expect(filterStudentRows(rows, "   ")).toBe(rows);
  });

  it("filters diacritic-insensitively and keeps the original order", () => {
    expect(filterStudentRows(rows, "anh").map((row) => row.name)).toEqual(["Lê Đức Anh"]);
    expect(filterStudentRows(rows, "duc").map((row) => row.name)).toEqual(["Lê Đức Anh"]);
    // "an" also sits inside "Trần": every row keeps its place.
    expect(filterStudentRows(rows, "an").map((row) => row.name)).toEqual(rows.map((r) => r.name));
  });

  it("returns an empty list when nothing matches", () => {
    expect(filterStudentRows(rows, "Trương")).toEqual([]);
  });
});
