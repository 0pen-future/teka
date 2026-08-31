import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { mockInvites } from "@/features/invitation/__tests__/invitation-handlers";
import {
  API_URL,
  CATALOG_VERSION,
  DEFAULT_CENTER_PERMISSIONS,
  DEFAULT_ROLES,
  fail,
} from "@/test/msw/handlers";
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
  it("renders API labels per role with every row assignable", async () => {
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
    const user = userEvent.setup();

    // Both keys live in admin-stub resources, folded into the shared tab.
    await user.click(await screen.findByRole("tab", { name: "Quản trị" }));

    // Labels come from the API catalog, one checkbox per role column.
    const auditCell = await screen.findByRole("checkbox", {
      name: "Xem nhật ký hoạt động — Giáo viên",
    });
    expect(auditCell).toBeChecked();
    expect(auditCell).toBeEnabled();
    expect(
      screen.getByRole("checkbox", { name: "Xem nhật ký hoạt động — Học vụ" }),
    ).not.toBeChecked();

    // The dual-life restriction retired with the can_send_reports column:
    // reports.send is a plain grantable key on role sets too.
    for (const role of ["Giáo viên", "Học vụ", "Trợ giảng"]) {
      expect(screen.getByRole("checkbox", { name: `Gửi báo cáo học phí — ${role}` })).toBeEnabled();
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

    await user.click(await screen.findByRole("tab", { name: "Quản trị" }));
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
    // The CAS pair rides along: the catalog generation and the role's
    // assignment version from the read model the edit was composed on.
    expect(received).toEqual({
      permissions: ["audit.read", "dashboard.view"],
      catalog_version: CATALOG_VERSION,
      assignment_version: HOC_VU.assignment_version,
    });
  });

  it("renders one tab per resource with admin stubs folded into Quản trị", async () => {
    mockCenterMe(makeCenterMeOwner({ members: [ownerSelf()] }));
    mockCenterPermissions(makeCenterPermissions());
    renderPermissionsPage();
    const user = userEvent.setup();

    // Business resources tab out in catalog order; the first one is active
    // by default, so only its rows are on screen.
    const classesTab = await screen.findByRole("tab", { name: "Lớp học" });
    expect(classesTab).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("checkbox", { name: "Tạo lớp học — Giáo viên" })).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Chốt kỳ học phí — Giáo viên" })).toBeNull();

    await user.click(screen.getByRole("tab", { name: "Học phí" }));
    expect(
      screen.getByRole("checkbox", { name: "Chốt kỳ học phí — Giáo viên" }),
    ).toBeInTheDocument();
    // Scope keys carry the high-risk marker inside their resource group.
    expect(screen.getAllByText("Rủi ro cao").length).toBeGreaterThan(0);

    // Admin stub resources share one tab, each keeping its own heading.
    await user.click(screen.getByRole("tab", { name: "Quản trị" }));
    expect(screen.getByRole("heading", { name: "Nhật ký" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Dashboard" })).toBeInTheDocument();
  });

  it("keeps an unsaved draft when switching tabs and marks its tab dirty", async () => {
    mockCenterMe(makeCenterMeOwner({ members: [ownerSelf()] }));
    mockCenterPermissions(makeCenterPermissions());
    renderPermissionsPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("checkbox", { name: "Tạo lớp học — Học vụ" }));
    await user.click(screen.getByRole("tab", { name: "Học phí" }));
    // The edited tab flags its pending draft while another tab is open — in
    // its accessible name, so assistive tech hears it too.
    const dirtyTab = screen.getByRole("tab", { name: /Lớp học.*có thay đổi chưa lưu/ });

    await user.click(dirtyTab);
    expect(screen.getByRole("checkbox", { name: "Tạo lớp học — Học vụ" })).toBeChecked();
    // The dirty role's save button stays live across the round-trip.
    expect(screen.getAllByRole("button", { name: "Lưu" })[1]).toBeEnabled();
  });

  it("asks for confirmation with the affected-member count before a high-risk save", async () => {
    const memberA = makeMember({ full_name: "Giáo Viên A" });
    mockCenterMe(makeCenterMeOwner({ members: [ownerSelf(), memberA] }));
    mockCenterPermissions(
      makeCenterPermissions({
        members: [makeMemberPermissions(memberA, { role_id: HOC_VU.id, role_key: HOC_VU.key })],
      }),
    );
    let puts = 0;
    server.use(
      http.put(`${API_URL}/centers/me/roles/:roleId/permissions`, () => {
        puts += 1;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderPermissionsPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("tab", { name: "Học phí" }));
    await user.click(await screen.findByRole("checkbox", { name: "Chốt kỳ học phí — Học vụ" }));
    const saveButtons = screen.getAllByRole("button", { name: "Lưu" });
    const hocVuSave = saveButtons[1];
    if (!hocVuSave) {
      throw new Error("missing the Học vụ save button");
    }
    await user.click(hocVuSave);

    // The confirmation summarizes the high-risk keys and who is affected.
    const confirm = await screen.findByRole("dialog");
    expect(within(confirm).getByText(/Chốt kỳ học phí/)).toBeInTheDocument();
    expect(within(confirm).getByText(/1 thành viên/)).toBeInTheDocument();
    await user.click(within(confirm).getByRole("button", { name: "Hủy" }));
    expect(puts).toBe(0);

    await user.click(hocVuSave);
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", { name: "Xác nhận" }),
    );
    expect(await screen.findByText("Đã lưu quyền cho vai trò Học vụ")).toBeInTheDocument();
    expect(puts).toBe(1);
  });

  it("keeps the draft and refetches on a stale-version 409 without auto-retrying", async () => {
    mockCenterMe(makeCenterMeOwner({ members: [ownerSelf()] }));
    const matrix = mockCenterPermissions(
      makeCenterPermissions(),
      makeCenterPermissions({
        roles: [
          DEFAULT_CENTER_PERMISSIONS.roles[0]!,
          { ...HOC_VU, permissions: ["classes.create"], assignment_version: 2 },
          DEFAULT_CENTER_PERMISSIONS.roles[2]!,
        ],
      }),
    );
    const conflictMessage = "Cấu hình quyền đã thay đổi từ lần tải trước, hãy tải lại rồi lưu lại";
    let puts = 0;
    server.use(
      http.put(`${API_URL}/centers/me/roles/:roleId/permissions`, () => {
        puts += 1;
        return HttpResponse.json(fail("CONFLICT", conflictMessage), { status: 409 });
      }),
    );
    renderPermissionsPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("tab", { name: "Quản trị" }));
    const auditBox = await screen.findByRole("checkbox", {
      name: "Xem nhật ký hoạt động — Học vụ",
    });
    await user.click(auditBox);
    const hocVuSave = screen.getAllByRole("button", { name: "Lưu" })[1];
    if (!hocVuSave) {
      throw new Error("missing the Học vụ save button");
    }
    await user.click(hocVuSave);

    // The API's conflict message surfaces verbatim and the read model
    // refetches, but the owner's draft stays for a reviewed re-save.
    expect(await screen.findByText(conflictMessage)).toBeInTheDocument();
    await waitFor(() => expect(matrix.calls).toBe(2));
    expect(puts).toBe(1);
    expect(screen.getByRole("checkbox", { name: "Xem nhật ký hoạt động — Học vụ" })).toBeChecked();
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
    // The precedence rule is stated where overrides are edited.
    expect(within(dialog).getByText(/Chặn riêng luôn thắng/)).toBeInTheDocument();

    await user.selectOptions(
      within(dialog).getByRole("combobox", { name: "Quyền Xem nhật ký hoạt động" }),
      "deny",
    );
    expect(within(dialog).getByText("Chặn riêng", { selector: "span" })).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Đã lưu phân quyền")).toBeInTheDocument();
    // Overrides carry the same CAS pair, from the member's own row.
    expect(received).toEqual({
      grants: ["imports.run"],
      denies: ["audit.read"],
      catalog_version: CATALOG_VERSION,
      assignment_version: 1,
    });
  });
});
