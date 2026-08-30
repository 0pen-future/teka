import { describe, expect, it } from "vitest";

import { centerMeSchema, renameCenterInputSchema } from "../schemas/center-schemas";
import { centerPermissionsSchema, groupCatalog } from "../schemas/permission-schemas";

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
    // An older API without the field still parses, defaulting to false.
    expect(me.members[0]?.can_send_reports).toBe(false);
  });

  it("carries a member's send-reports flag when the API sends it", () => {
    const me = centerMeSchema.parse({
      center: { id: "c1", name: "Trung Tâm Bình Minh", is_owner: true },
      members: [
        {
          id: "t2",
          full_name: "Giáo Viên A",
          phone: "+84901000002",
          is_owner: false,
          can_send_reports: true,
        },
      ],
    });
    if (!("members" in me)) {
      throw new Error("expected the owner shape");
    }
    expect(me.members[0]?.can_send_reports).toBe(true);
  });

  it("parses the member-shaped GET /centers/me contract", () => {
    const me = centerMeSchema.parse({ center_name: "Trung Tâm Bình Minh" });
    if ("members" in me) {
      throw new Error("expected the member shape");
    }
    expect(me.center_name).toBe("Trung Tâm Bình Minh");
    // Same rollout default on the member's own flag.
    expect(me.can_send_reports).toBe(false);
  });

  it("carries the member's own send-reports flag when the API sends it", () => {
    const me = centerMeSchema.parse({ center_name: "Trung Tâm Bình Minh", can_send_reports: true });
    if ("members" in me) {
      throw new Error("expected the member shape");
    }
    expect(me.can_send_reports).toBe(true);
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

describe("centerPermissionsSchema", () => {
  const baseRole = { id: "r1", key: "hoc_vu", name: "Học vụ", permissions: [] };

  it("parses the structured v2 read model with CAS versions", () => {
    const model = centerPermissionsSchema.parse({
      catalog: [
        {
          key: "billing.close",
          label: "Chốt kỳ học phí",
          resource: "billing",
          action: "close",
          kind: "special",
          risk: "high",
          description: "Chốt kỳ học phí — khóa hóa đơn của kỳ.",
        },
      ],
      roles: [{ ...baseRole, assignment_version: 7 }],
      members: [],
      catalog_version: 2,
    });
    expect(model.catalog[0]?.kind).toBe("special");
    expect(model.catalog[0]?.risk).toBe("high");
    expect(model.roles[0]?.assignment_version).toBe(7);
    expect(model.catalog_version).toBe(2);
  });

  it("defaults the structured fields and versions when a rolled-back API omits them", () => {
    const model = centerPermissionsSchema.parse({
      catalog: [{ key: "audit.read", label: "Xem nhật ký hoạt động" }],
      roles: [baseRole],
      members: [
        {
          teacher_id: "t1",
          full_name: "Giáo Viên A",
          role_id: null,
          role_key: "",
          grants: [],
          denies: [],
        },
      ],
    });
    expect(model.catalog[0]?.kind).toBe("crud");
    expect(model.catalog[0]?.risk).toBe("low");
    // Version 0 is the API's "skip the CAS check" sentinel.
    expect(model.catalog_version).toBe(0);
    expect(model.roles[0]?.assignment_version).toBe(0);
    expect(model.members[0]?.assignment_version).toBe(0);
  });

  it("falls back to high risk for an unknown risk grade from a newer API", () => {
    const model = centerPermissionsSchema.parse({
      catalog: [{ key: "x.y", label: "X", kind: "weird", risk: "critical" }],
      roles: [],
      members: [],
      catalog_version: 3,
    });
    // Over-warning is the safe direction; unknown kind degrades to crud.
    expect(model.catalog[0]?.risk).toBe("high");
    expect(model.catalog[0]?.kind).toBe("crud");
  });
});

describe("groupCatalog", () => {
  it("groups by resource preserving registry order with Vietnamese headings", () => {
    const groups = groupCatalog(
      centerPermissionsSchema.parse({
        catalog: [
          { key: "classes.create", label: "Tạo lớp học", resource: "classes", action: "create" },
          {
            key: "classes.view_all",
            label: "Xem mọi lớp học",
            resource: "classes",
            action: "view_all",
            kind: "scope",
            risk: "high",
          },
          {
            key: "billing.close",
            label: "Chốt kỳ học phí",
            resource: "billing",
            action: "close",
            kind: "special",
            risk: "high",
          },
        ],
        roles: [],
        members: [],
        catalog_version: 2,
      }).catalog,
    );
    expect(groups.map((g) => g.label)).toEqual(["Lớp học", "Học phí"]);
    expect(groups[0]?.entries.map((e) => e.key)).toEqual(["classes.create", "classes.view_all"]);
  });
});
