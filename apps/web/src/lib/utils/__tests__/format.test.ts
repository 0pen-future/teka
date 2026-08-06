import { describe, expect, it } from "vitest";

import { formatMoney, formatPhoneLocal, formatSessionDate, nameInitial } from "../format";

describe("formatMoney", () => {
  it("renders zero with the đồng symbol", () => {
    // Intl's vi-VN currency format separates the amount and symbol with a
    // non-breaking space (U+00A0), not a regular space.
    expect(formatMoney(0)).toBe("0 ₫");
  });

  it("groups a million-scale value with no fraction digits", () => {
    expect(formatMoney(1_500_000)).toBe("1.500.000 ₫");
  });
});

describe("formatSessionDate", () => {
  it("renders the weekday and dd/MM for a valid date", () => {
    expect(formatSessionDate("2026-07-15")).toBe("Th 4, 15/07");
  });

  it("renders Sunday as CN", () => {
    expect(formatSessionDate("2026-07-19")).toBe("CN, 19/07");
  });

  it("passes an invalid date string through unchanged", () => {
    expect(formatSessionDate("not-a-date")).toBe("not-a-date");
  });
});

describe("formatPhoneLocal", () => {
  it("converts an E.164 Vietnamese number back to local form", () => {
    expect(formatPhoneLocal("+84912345678")).toBe("0912345678");
  });

  it("passes a number without the +84 prefix through unchanged", () => {
    expect(formatPhoneLocal("0912345678")).toBe("0912345678");
  });
});

describe("nameInitial", () => {
  it("takes the first letter of the last word — the Vietnamese given name", () => {
    expect(nameInitial("Nguyễn Thị Lan")).toBe("L");
  });

  it("handles a single-word name", () => {
    expect(nameInitial("Lan")).toBe("L");
  });

  it("returns an empty string for empty or whitespace-only input", () => {
    expect(nameInitial("")).toBe("");
    expect(nameInitial("   ")).toBe("");
  });
});
