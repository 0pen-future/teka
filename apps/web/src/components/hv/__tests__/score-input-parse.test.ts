import { describe, expect, it } from "vitest";

import { parseScoreInput } from "@/components/hv";

describe("parseScoreInput", () => {
  it.each([
    ["", null],
    ["  ", null],
    ["7", 7],
    ["7,5", 7.5],
    ["7.3", 7.5],
    ["7.2", 7],
    ["0", 0],
    ["10", 10],
    ["-1", "invalid"],
    ["12", 10],
    ["abc", "invalid"],
    [" 8 ", 8],
    ["7abc", "invalid"],
    ["7,5,5", "invalid"],
    ["1e1", "invalid"],
    ["9.75", 10],
  ] as const)("parses %j as %j", (raw, expected) => {
    expect(parseScoreInput(raw)).toBe(expected);
  });
});
