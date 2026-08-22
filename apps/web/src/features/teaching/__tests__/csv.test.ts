import { describe, expect, it } from "vitest";

import { toCsv } from "../lib/csv";

describe("toCsv", () => {
  it("prefixes a BOM, quotes every cell, joins with semicolons and newlines", () => {
    const csv = toCsv([
      ["Buổi", "Điểm TB"],
      ["Th 4, 05/08", 7.5],
    ]);
    expect(csv).toBe('\uFEFF"Buổi";"Điểm TB"\n"Th 4, 05/08";"7.5"');
  });

  it("escapes embedded quotes and renders null/undefined as empty cells", () => {
    const csv = toCsv([['nói "được"', null, undefined, 0]]);
    expect(csv).toBe('\uFEFF"nói ""được""";"";"";"0"');
  });
});
