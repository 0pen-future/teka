import { describe, expect, it } from "vitest";

import { findFoldedMatch, foldVietnamese } from "../vietnamese";

describe("foldVietnamese", () => {
  it("strips diacritics, maps đ to d, and lowercases", () => {
    expect(foldVietnamese("Trần Đức Ánh")).toBe("tran duc anh");
    expect(foldVietnamese("Nguyễn Đức")).toBe("nguyen duc");
  });

  it("folds NFD input the same as NFC", () => {
    expect(foldVietnamese("Nguyẽn")).toBe(foldVietnamese("Nguyễn"));
  });

  it("leaves plain ASCII untouched apart from case", () => {
    expect(foldVietnamese("An B")).toBe("an b");
  });
});

describe("findFoldedMatch", () => {
  it("returns the original-string range of an unaccented query", () => {
    expect(findFoldedMatch("Nguyễn Văn An", "nguyen")).toEqual({ start: 0, end: 6 });
  });

  it("matches accented queries against accented text", () => {
    expect(findFoldedMatch("Nguyễn Văn An", "Văn")).toEqual({ start: 7, end: 10 });
  });

  it("maps offsets back correctly when the text is stored as NFD", () => {
    // "ễ" as e + two combining marks: the folded length (6) is shorter than
    // the original (8), so the end offset must land after the marks.
    const nfd = "Nguyễn Van";
    const match = findFoldedMatch(nfd, "nguyen")!;
    expect(nfd.slice(match.start, match.end)).toBe("Nguyễn");
  });

  it("pulls trailing combining marks into the range so glyphs stay whole", () => {
    const nfd = "Nguyễn";
    const match = findFoldedMatch(nfd, "nguye")!;
    expect(nfd.slice(match.start, match.end)).toBe("Nguyễ");
  });

  it("matches đ with d in either direction", () => {
    expect(findFoldedMatch("Đặng Thu", "dang")).toEqual({ start: 0, end: 4 });
    expect(findFoldedMatch("Dang Thu", "đặng")).toEqual({ start: 0, end: 4 });
  });

  it("returns null when nothing matches or the query is blank", () => {
    expect(findFoldedMatch("Nguyễn Văn An", "Trương")).toBeNull();
    expect(findFoldedMatch("Nguyễn Văn An", "")).toBeNull();
    expect(findFoldedMatch("Nguyễn Văn An", "   ")).toBeNull();
  });

  it("trims the query before matching", () => {
    expect(findFoldedMatch("Nguyễn Văn An", "  van ")).toEqual({ start: 7, end: 10 });
  });
});
