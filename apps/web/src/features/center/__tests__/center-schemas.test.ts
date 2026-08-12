import { describe, expect, it } from "vitest";

import {
  centerMeSchema,
  joinCenterInputSchema,
  renameCenterInputSchema,
} from "../schemas/center-schemas";

describe("centerMeSchema", () => {
  it("parses the GET /centers/me contract", () => {
    const me = centerMeSchema.parse({
      center: { id: "c1", name: "Trung Tâm Bình Minh", is_owner: true },
      members: [
        { id: "t1", full_name: "Cô Lan", phone: "+84901000001", is_owner: true },
        { id: "t2", full_name: "Giáo Viên A", phone: "+84901000002", is_owner: false },
      ],
    });
    expect(me.center.is_owner).toBe(true);
    expect(me.members).toHaveLength(2);
  });

  it("rejects a payload missing is_owner — the role-gating anchor", () => {
    expect(() =>
      centerMeSchema.parse({
        center: { id: "c1", name: "X" },
        members: [],
      }),
    ).toThrow();
  });
});

describe("joinCenterInputSchema", () => {
  it("accepts both local and E.164 phone forms", () => {
    expect(joinCenterInputSchema.parse({ owner_phone: "0901234567" }).owner_phone).toBe(
      "0901234567",
    );
    expect(joinCenterInputSchema.parse({ owner_phone: "+84901234567" }).owner_phone).toBe(
      "+84901234567",
    );
  });

  it("rejects garbage before it round-trips", () => {
    expect(joinCenterInputSchema.safeParse({ owner_phone: "12345" }).success).toBe(false);
    expect(joinCenterInputSchema.safeParse({ owner_phone: "" }).success).toBe(false);
  });
});

describe("renameCenterInputSchema", () => {
  it("trims and requires a non-empty name", () => {
    expect(renameCenterInputSchema.parse({ name: "  Bình Minh  " }).name).toBe("Bình Minh");
    expect(renameCenterInputSchema.safeParse({ name: "   " }).success).toBe(false);
  });
});
