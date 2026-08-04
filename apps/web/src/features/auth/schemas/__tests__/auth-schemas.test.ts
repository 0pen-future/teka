import { describe, expect, it } from "vitest";

import { normalizePhone } from "../auth-schemas";

describe("normalizePhone", () => {
  it("converts a local 0-prefixed number to E.164", () => {
    expect(normalizePhone("0912345678")).toBe("+84912345678");
  });

  it("passes an already-E.164 number through unchanged", () => {
    // Guards the wire format: if the server ever starts sending E.164 phones
    // back through a form default, normalization must stay a no-op instead
    // of double-prefixing.
    expect(normalizePhone("+84912345678")).toBe("+84912345678");
  });
});
