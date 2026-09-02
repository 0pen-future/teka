import { describe, expect, it } from "vitest";

import { findDuplicateIndexes, parsePastedComponents } from "../lib/score-set-components";

describe("parsePastedComponents", () => {
  it("splits on newlines, commas and semicolons and trims blanks", () => {
    expect(parsePastedComponents("Miệng, 15 phút;Giữa kỳ\n\n  Cuối kỳ  \n")).toEqual({
      names: ["Miệng", "15 phút", "Giữa kỳ", "Cuối kỳ"],
      truncated: false,
    });
  });

  it("returns an empty list for whitespace-only input", () => {
    expect(parsePastedComponents("  \n , ; ")).toEqual({ names: [], truncated: false });
  });

  it("keeps only the first ten names and flags truncation", () => {
    const text = Array.from({ length: 12 }, (_, i) => `Cột ${i + 1}`).join("\n");
    const result = parsePastedComponents(text);
    expect(result.names).toHaveLength(10);
    expect(result.names[9]).toBe("Cột 10");
    expect(result.truncated).toBe(true);
  });

  it("keeps duplicates for the caller to report", () => {
    expect(parsePastedComponents("Miệng\nmiệng").names).toEqual(["Miệng", "miệng"]);
  });
});

describe("findDuplicateIndexes", () => {
  it("reports later occurrences only, case-insensitively and trimmed", () => {
    expect([...findDuplicateIndexes(["Miệng", " miệng ", "15 phút", "MIỆNG"])]).toEqual([1, 3]);
  });

  it("ignores blank names", () => {
    expect(findDuplicateIndexes(["", " ", "Miệng"]).size).toBe(0);
  });
});
