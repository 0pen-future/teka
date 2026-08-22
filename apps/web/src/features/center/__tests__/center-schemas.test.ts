import { describe, expect, it } from "vitest";

import { centerMeSchema, renameCenterInputSchema } from "../schemas/center-schemas";

describe("centerMeSchema", () => {
  it("parses the owner-shaped GET /centers/me contract", () => {
    const me = centerMeSchema.parse({
      center: { id: "c1", name: "Trung Tâm Bình Minh", is_owner: true },
      members: [
        { id: "t1", full_name: "Cô Lan", phone: "+84901000001", is_owner: true },
        { id: "t2", full_name: "Giáo Viên A", phone: "+84901000002", is_owner: false },
      ],
    });
    if (!("members" in me)) {
      throw new Error("expected the owner shape");
    }
    expect(me.center.is_owner).toBe(true);
    expect(me.members).toHaveLength(2);
  });

  it("parses the member-shaped GET /centers/me contract", () => {
    const me = centerMeSchema.parse({ center_name: "Trung Tâm Bình Minh" });
    if ("members" in me) {
      throw new Error("expected the member shape");
    }
    expect(me.center_name).toBe("Trung Tâm Bình Minh");
  });

  it("rejects a payload matching neither role shape", () => {
    expect(() => centerMeSchema.parse({ center: { id: "c1", name: "X" } })).toThrow();
    expect(centerMeSchema.safeParse({}).success).toBe(false);
  });
});

describe("renameCenterInputSchema", () => {
  it("trims and requires a non-empty name", () => {
    expect(renameCenterInputSchema.parse({ name: "  Bình Minh  " }).name).toBe("Bình Minh");
    expect(renameCenterInputSchema.safeParse({ name: "   " }).success).toBe(false);
  });
});
