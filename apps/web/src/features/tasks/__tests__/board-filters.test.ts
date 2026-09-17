import { describe, expect, it } from "vitest";

import { assigneeChips, isFiltering } from "../lib/board-filters";

describe("assigneeChips", () => {
  const members = [
    { teacher_id: "t1", display_name: "Bình" },
    { teacher_id: "t2", display_name: "An" },
  ];

  it("joins counts against the member directory and drops zero-count entries", () => {
    const chips = assigneeChips(
      [
        { teacher_id: "t1", count: 3 },
        { teacher_id: "t2", count: 0 },
      ],
      members,
    );
    expect(chips).toEqual([{ teacherId: "t1", label: "Bình", count: 3 }]);
  });

  it("sorts by count descending, then by label", () => {
    const chips = assigneeChips(
      [
        { teacher_id: "t1", count: 2 },
        { teacher_id: "t2", count: 5 },
      ],
      members,
    );
    expect(chips.map((chip) => chip.teacherId)).toEqual(["t2", "t1"]);
  });

  it("falls back to a placeholder label for a teacher missing from the directory", () => {
    const chips = assigneeChips([{ teacher_id: "ghost", count: 1 }], members);
    expect(chips).toEqual([{ teacherId: "ghost", label: "Không rõ", count: 1 }]);
  });
});

describe("isFiltering", () => {
  it("is false for the default (unfiltered) state", () => {
    expect(isFiltering({ filter: "all", assignee: "" })).toBe(false);
  });

  it("is true once the status filter narrows the board", () => {
    expect(isFiltering({ filter: "overdue", assignee: "" })).toBe(true);
  });

  it("is true once an assignee narrows the board, even with filter left at 'all'", () => {
    expect(isFiltering({ filter: "all", assignee: "t1" })).toBe(true);
  });
});
