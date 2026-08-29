import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { mockInvites } from "@/features/invitation/__tests__/invitation-handlers";
import { API_URL, DEFAULT_CENTER_PERMISSIONS, DEFAULT_ROLES } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { CenterPage } from "../pages/center-page";
import { CenterPermissionsPage } from "../pages/center-permissions-page";
import {
  makeCenterMeMember,
  makeCenterMeOwner,
  makeCenterPermissions,
  makeMember,
  makeMemberPermissions,
  mockCenterMe,
  mockCenterPermissions,
} from "./center-handlers";

const { giaoVien: GIAO_VIEN, hocVu: HOC_VU } = DEFAULT_ROLES;

function renderCenter() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<CenterPage />, { route: "/center", path: "/center" });
}

function renderPermissionsPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<CenterPermissionsPage />, {
    route: "/center/permissions",
    path: "/center/permissions",
    extraRoutes: [{ path: "/", element: <div>Trang tổng quan</div> }],
  });
}

function ownerSelf() {
  return makeMember({
    id: testPrimaryTeacher.id,
    full_name: testPrimaryTeacher.full_name,
    phone: testPrimaryTeacher.phone,
    is_owner: true,
  });
}

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("PermissionMatrix on the owner permissions page", () => {
  it("renders API labels per role and keeps the reports.send row disabled", async () => {
    mockCenterMe(makeCenterMeOwner({ members: [ownerSelf()] }));
    mockCenterPermissions(
      makeCenterPermissions({
        roles: [
          { ...GIAO_VIEN, permissions: ["audit.read"] },
          ...DEFAULT_CENTER_PERMISSIONS.roles.slice(1),
        ],
      }),
    );
    renderPermissionsPage();

    // Labels come from the API catalog, one checkbox per role column.
    const auditCell = await screen.findByRole("checkbox", {
      name: "Xem nhật ký hoạt động — Giáo viên",
    });
    expect(auditCell).toBeChecked();
    expect(auditCell).toBeEnabled();
    expect(
      screen.getByRole("checkbox", { name: "Xem nhật ký hoạt động — Học vụ" }),
    ).not.toBeChecked();

    // The dual-life restriction: reports.send is per-member only for now.
    for (const role of ["Giáo viên", "Học vụ", "Trợ giảng"]) {
      expect(
        screen.getByRole("checkbox", { name: `Gửi báo cáo học phí — ${role}` }),
      ).toBeDisabled();
    }
  });

  it("saves a role's full checked set through the role save button", async () => {
    mockCenterMe(makeCenterMeOwner({ members: [ownerSelf()] }));
    mockCenterPermissions(makeCenterPermissions());
    let savedRoleId = "";
    let received: unknown;
    server.use(
      http.put(`${API_URL}/centers/me/roles/:roleId/permissions`, async ({ params, request }) => {
        savedRoleId = String(params.roleId);
        received = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderPermissionsPage();
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("checkbox", { name: "Xem nhật ký hoạt động — Học vụ" }),
    );
    await user.click(screen.getByRole("checkbox", { name: "Xem dashboard trung tâm — Học vụ" }));
    // Save buttons follow role column order; only the dirty one is enabled.
    const saveButtons = screen.getAllByRole("button", { name: "Lưu" });
    expect(saveButtons[0]).toBeDisabled();
    const hocVuSave = saveButtons[1];
    if (!hocVuSave) {
      throw new Error("missing the Học vụ save button");
    }
    await user.click(hocVuSave);

    expect(await screen.findByText("Đã lưu quyền cho vai trò Học vụ")).toBeInTheDocument();
    expect(savedRoleId).toBe(HOC_VU.id);
    expect(received).toEqual({ permissions: ["audit.read", "dashboard.view"] });
  });

  it("redirects a non-owner deep link to the dashboard without fetching the matrix", async () => {
    mockCenterMe(makeCenterMeMember());
    const matrix = mockCenterPermissions(makeCenterPermissions());
    const { router } = renderPermissionsPage();

    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
    expect(await screen.findByText("Trang tổng quan")).toBeInTheDocument();
    // The owner-only read model never fires for a redirected member.
    expect(matrix.calls).toBe(0);
  });
});

describe("MemberPermissionsDialog", () => {
  it("assigns a role immediately from the role select", async () => {
    mockInvites([]);
    const memberA = makeMember({ full_name: "Giáo Viên A" });
    mockCenterMe(makeCenterMeOwner({ members: [ownerSelf(), memberA] }));
    mockCenterPermissions(makeCenterPermissions({ members: [makeMemberPermissions(memberA)] }));
    let targetId = "";
    let received: unknown;
    server.use(
      http.put(`${API_URL}/centers/me/members/:teacherId/role`, async ({ params, request }) => {
        targetId = String(params.teacherId);
        received = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderCenter();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Phân quyền cho Giáo Viên A" }));
    const dialog = await screen.findByRole("dialog");
    // A pre-RBAC membership holds no role: the placeholder is selected.
    const roleSelect = await within(dialog).findByRole("combobox", { name: "Vai trò" });
    expect(roleSelect).toHaveValue("");
    await user.selectOptions(roleSelect, HOC_VU.id);

    expect(await screen.findByText("Đã đổi vai trò")).toBeInTheDocument();
    expect(targetId).toBe(memberA.id);
    expect(received).toEqual({ role_id: HOC_VU.id });
  });

  it("shows the effective source per key and saves a deny of a role permission", async () => {
    mockInvites([]);
    const memberA = makeMember({ full_name: "Giáo Viên A" });
    mockCenterMe(makeCenterMeOwner({ members: [ownerSelf(), memberA] }));
    mockCenterPermissions(
      makeCenterPermissions({
        roles: [
          { ...GIAO_VIEN, permissions: ["audit.read"] },
          ...DEFAULT_CENTER_PERMISSIONS.roles.slice(1),
        ],
        members: [
          makeMemberPermissions(memberA, {
            role_id: GIAO_VIEN.id,
            role_key: GIAO_VIEN.key,
            grants: ["imports.run"],
          }),
        ],
      }),
    );
    let received: unknown;
    server.use(
      http.put(`${API_URL}/centers/me/members/:teacherId/overrides`, async ({ request }) => {
        received = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderCenter();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Phân quyền cho Giáo Viên A" }));
    const dialog = await screen.findByRole("dialog");
    // Effective sources: role-inherited vs. individually granted.
    expect(await within(dialog).findByText("Từ vai trò")).toBeInTheDocument();
    expect(within(dialog).getByText("Cấp riêng", { selector: "span" })).toBeInTheDocument();

    await user.selectOptions(
      within(dialog).getByRole("combobox", { name: "Quyền Xem nhật ký hoạt động" }),
      "deny",
    );
    expect(within(dialog).getByText("Chặn riêng", { selector: "span" })).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Đã lưu phân quyền")).toBeInTheDocument();
    expect(received).toEqual({ grants: ["imports.run"], denies: ["audit.read"] });
  });
});
