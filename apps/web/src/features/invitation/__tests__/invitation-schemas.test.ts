import { describe, expect, it } from "vitest";

import {
  acceptInviteFormSchema,
  createInviteInputSchema,
  createInviteResponseSchema,
  invitationSchema,
  invitePreviewSchema,
} from "../schemas/invitation-schemas";

describe("createInviteInputSchema", () => {
  it("accepts both local and E.164 phone forms", () => {
    expect(createInviteInputSchema.parse({ phone: "0901234567" }).phone).toBe("0901234567");
    expect(createInviteInputSchema.parse({ phone: "+84901234567" }).phone).toBe("+84901234567");
  });

  it("rejects garbage before it round-trips", () => {
    expect(createInviteInputSchema.safeParse({ phone: "12345" }).success).toBe(false);
    expect(createInviteInputSchema.safeParse({ phone: "" }).success).toBe(false);
  });
});

describe("createInviteResponseSchema", () => {
  it("parses the POST /centers/me/invitations contract", () => {
    const response = createInviteResponseSchema.parse({
      id: "i1",
      phone: "+84901234567",
      expires_at: "2026-08-19T10:00:00Z",
      link: "https://app.teka.dev/invite/abc",
      dm_status: "sent",
    });
    expect(response.dm_status).toBe("sent");
  });
});

describe("invitationSchema", () => {
  it("parses every server-derived status value", () => {
    for (const status of ["pending", "accepted", "revoked", "expired"] as const) {
      expect(
        invitationSchema.parse({
          id: "i1",
          phone: "+84901234567",
          status,
          expires_at: "2026-08-19T10:00:00Z",
          created_at: "2026-08-12T10:00:00Z",
        }).status,
      ).toBe(status);
    }
  });
});

describe("invitePreviewSchema", () => {
  it("parses the POST /invitations/preview contract", () => {
    const preview = invitePreviewSchema.parse({
      center_name: "Trung Tâm Bình Minh",
      phone_masked: "+84******567",
    });
    expect(preview.center_name).toBe("Trung Tâm Bình Minh");
  });
});

describe("acceptInviteFormSchema", () => {
  it("rejects a mismatched confirm password", () => {
    const result = acceptInviteFormSchema.safeParse({
      full_name: "Cô Lan",
      password: "long-enough-password",
      confirm_password: "different-password",
    });
    expect(result.success).toBe(false);
  });

  it("accepts a matching password pair", () => {
    const result = acceptInviteFormSchema.safeParse({
      full_name: "Cô Lan",
      password: "long-enough-password",
      confirm_password: "long-enough-password",
    });
    expect(result.success).toBe(true);
  });

  it("enforces the server's password length bound client-side", () => {
    expect(
      acceptInviteFormSchema.safeParse({
        full_name: "Cô Lan",
        password: "short",
        confirm_password: "short",
      }).success,
    ).toBe(false);
  });
});
