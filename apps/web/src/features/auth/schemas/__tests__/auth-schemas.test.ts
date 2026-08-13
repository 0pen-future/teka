import { describe, expect, it } from "vitest";

import {
  forgotPasswordInputSchema,
  normalizePhone,
  resetPasswordFormSchema,
} from "../auth-schemas";

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

describe("forgotPasswordInputSchema", () => {
  it("normalizes a local phone to E.164 on parse", () => {
    expect(forgotPasswordInputSchema.parse({ phone: "0912345678" }).phone).toBe("+84912345678");
  });

  it("rejects an invalid phone", () => {
    expect(forgotPasswordInputSchema.safeParse({ phone: "123" }).success).toBe(false);
  });
});

describe("resetPasswordFormSchema", () => {
  it("rejects a mismatched confirm password", () => {
    expect(
      resetPasswordFormSchema.safeParse({
        password: "long-enough-password",
        confirm_password: "different-password",
      }).success,
    ).toBe(false);
  });

  it("accepts a matching password pair", () => {
    expect(
      resetPasswordFormSchema.safeParse({
        password: "long-enough-password",
        confirm_password: "long-enough-password",
      }).success,
    ).toBe(true);
  });

  it("enforces the server's password length bound client-side", () => {
    expect(
      resetPasswordFormSchema.safeParse({ password: "short", confirm_password: "short" }).success,
    ).toBe(false);
  });
});
