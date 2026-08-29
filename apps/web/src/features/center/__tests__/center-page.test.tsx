import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { mockInvites } from "@/features/invitation/__tests__/invitation-handlers";
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { CenterPage } from "../pages/center-page";
import {
  makeCenterMeMember,
  makeCenterMeOwner,
  makeCenterPermissions,
  makeMember,
  makeMemberPermissions,
  mockCenterMe,
  mockCenterPermissions,
} from "./center-handlers";

function renderCenter() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<CenterPage />, { route: "/center", path: "/center" });
}

/** The signed-in teacher as a roster row, so self-detection has an anchor. */
function selfMember(isOwner: boolean) {
  return makeMember({
    id: testPrimaryTeacher.id,
    full_name: testPrimaryTeacher.full_name,
    phone: testPrimaryTeacher.phone,
    is_owner: isOwner,
  });
}

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("CenterPage — owner", () => {
  function mockSharedCenter() {
    mockInvites([]);
    return mockCenterMe(
      makeCenterMeOwner({
        members: [selfMember(true), makeMember({ full_name: "Giáo Viên A" })],
      }),
    );
  }

  it("shows the center header, the member roster, and the invite section", async () => {
    mockSharedCenter();
    renderCenter();

    expect(await screen.findByText("Trung Tâm Bình Minh")).toBeInTheDocument();
    expect(screen.getByText("Chủ trung tâm")).toBeInTheDocument();
    expect(screen.getByText("Giáo Viên A")).toBeInTheDocument();
    expect(await screen.findByLabelText("Số điện thoại")).toBeInTheDocument();
  });

  it("renames the center through the owner-only dialog", async () => {
    mockSharedCenter();
    let received: { name?: string } = {};
    server.use(
      http.patch(`${API_URL}/centers/me`, async ({ request }) => {
        received = (await request.json()) as typeof received;
        return HttpResponse.json(
          ok(
            makeCenterMeOwner({
              center: { id: "c1", name: "Trung Tâm Hoa Sen", is_owner: true },
              members: [selfMember(true), makeMember({ full_name: "Giáo Viên A" })],
            }),
          ),
        );
      }),
    );
    renderCenter();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Đổi tên trung tâm" }));
    const input = await screen.findByLabelText("Tên trung tâm");
    await user.clear(input);
    await user.type(input, "Trung Tâm Hoa Sen");
    await user.click(screen.getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Đã đổi tên trung tâm")).toBeInTheDocument();
    expect(received).toEqual({ name: "Trung Tâm Hoa Sen" });
    expect(await screen.findByText("Trung Tâm Hoa Sen")).toBeInTheDocument();
  });

  it("disables a member's login after a confirm that says the data stays", async () => {
    // Post-refetch roster no longer contains the disabled teacher.
    mockInvites([]);
    mockCenterMe(
      makeCenterMeOwner({
        members: [selfMember(true), makeMember({ full_name: "Giáo Viên A" })],
      }),
      makeCenterMeOwner({ members: [selfMember(true)] }),
    );
    let deletedPath = "";
    server.use(
      http.delete(`${API_URL}/centers/me/members/:teacherId`, ({ params }) => {
        deletedPath = String(params.teacherId);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    renderCenter();
    const user = userEvent.setup();

    const row = (await screen.findByText("Giáo Viên A")).closest("li");
    if (!row) {
      throw new Error("member row not rendered as a list item");
    }
    await user.click(within(row).getByRole("button", { name: "Xoá Giáo Viên A" }));
    expect(await screen.findByText(/ở lại trung tâm/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Vô hiệu hoá đăng nhập" }));

    expect(await screen.findByText("Đã vô hiệu hoá đăng nhập")).toBeInTheDocument();
    expect(deletedPath).not.toBe("");
    await waitFor(() => expect(screen.queryByText("Giáo Viên A")).not.toBeInTheDocument());
  });

  it("surfaces a failure toast when disabling a member errors", async () => {
    mockSharedCenter();
    server.use(
      http.delete(`${API_URL}/centers/me/members/:teacherId`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "boom"), { status: 500 }),
      ),
    );
    renderCenter();
    const user = userEvent.setup();

    const row = (await screen.findByText("Giáo Viên A")).closest("li");
    if (!row) {
      throw new Error("member row not rendered as a list item");
    }
    await user.click(within(row).getByRole("button", { name: "Xoá Giáo Viên A" }));
    await user.click(await screen.findByRole("button", { name: "Vô hiệu hoá đăng nhập" }));

    expect(await screen.findByText("Có lỗi xảy ra, thử lại sau")).toBeInTheDocument();
    // Roster row + the still-open confirm dialog both name the member.
    expect(screen.getAllByText("Giáo Viên A").length).toBeGreaterThan(0);
  });

  it("shows a server error on the rename form without closing the dialog", async () => {
    mockSharedCenter();
    server.use(
      http.patch(`${API_URL}/centers/me`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "Không đổi được tên"), { status: 500 }),
      ),
    );
    renderCenter();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Đổi tên trung tâm" }));
    await user.click(screen.getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Không đổi được tên")).toBeInTheDocument();
    expect(screen.getByLabelText("Tên trung tâm")).toBeInTheDocument();
  });

  it("converges on 404 when the member is already disabled — no red error", async () => {
    mockSharedCenter();
    server.use(
      http.delete(`${API_URL}/centers/me/members/:teacherId`, () =>
        HttpResponse.json(
          { success: false, error: { code: "NOT_FOUND", message: "member not found" } },
          { status: 404 },
        ),
      ),
    );
    renderCenter();
    const user = userEvent.setup();

    const row = (await screen.findByText("Giáo Viên A")).closest("li");
    if (!row) {
      throw new Error("member row not rendered as a list item");
    }
    await user.click(within(row).getByRole("button", { name: "Xoá Giáo Viên A" }));
    await user.click(await screen.findByRole("button", { name: "Vô hiệu hoá đăng nhập" }));

    expect(await screen.findByText("Đã vô hiệu hoá đăng nhập")).toBeInTheDocument();
    expect(screen.queryByText("Something went wrong")).not.toBeInTheDocument();
  });

  it("never offers a remove button on the owner's own row", async () => {
    mockSharedCenter();
    renderCenter();

    await screen.findByText("Giáo Viên A");
    expect(
      screen.queryByRole("button", { name: `Xoá ${testPrimaryTeacher.full_name}` }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: `Phân quyền cho ${testPrimaryTeacher.full_name}`,
      }),
    ).not.toBeInTheDocument();
  });

  it("grants reports.send through the permissions dialog and shows the badge after refetch", async () => {
    mockInvites([]);
    const memberA = makeMember({ full_name: "Giáo Viên A" });
    mockCenterMe(
      makeCenterMeOwner({ members: [selfMember(true), memberA] }),
      makeCenterMeOwner({
        members: [selfMember(true), { ...memberA, can_send_reports: true }],
      }),
    );
    // A single scripted payload: the dialog's own mount refetch would consume
    // a "post-save" second payload early and erase the draft's dirtiness.
    mockCenterPermissions(makeCenterPermissions({ members: [makeMemberPermissions(memberA)] }));
    let targetId = "";
    let received: unknown;
    server.use(
      http.put(
        `${API_URL}/centers/me/members/:teacherId/overrides`,
        async ({ params, request }) => {
          targetId = String(params.teacherId);
          received = await request.json();
          return new HttpResponse(null, { status: 204 });
        },
      ),
    );
    renderCenter();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Phân quyền cho Giáo Viên A" }));
    const dialog = await screen.findByRole("dialog");
    await user.selectOptions(
      await within(dialog).findByRole("combobox", { name: "Quyền Gửi báo cáo học phí" }),
      "grant",
    );
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Đã lưu phân quyền")).toBeInTheDocument();
    expect(targetId).toBe(memberA.id);
    expect(received).toEqual({ grants: ["reports.send"], denies: [] });
    expect(await screen.findByText("Thư ký gửi báo cáo")).toBeInTheDocument();
  });

  it("clears the reports.send grant and drops the badge after refetch", async () => {
    mockInvites([]);
    const memberA = makeMember({ full_name: "Giáo Viên A", can_send_reports: true });
    mockCenterMe(
      makeCenterMeOwner({ members: [selfMember(true), memberA] }),
      makeCenterMeOwner({
        members: [selfMember(true), { ...memberA, can_send_reports: false }],
      }),
    );
    mockCenterPermissions(
      makeCenterPermissions({
        members: [makeMemberPermissions(memberA, { grants: ["reports.send"] })],
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

    expect(await screen.findByText("Thư ký gửi báo cáo")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Phân quyền cho Giáo Viên A" }));
    const dialog = await screen.findByRole("dialog");
    await user.selectOptions(
      await within(dialog).findByRole("combobox", { name: "Quyền Gửi báo cáo học phí" }),
      "inherit",
    );
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Đã lưu phân quyền")).toBeInTheDocument();
    expect(received).toEqual({ grants: [], denies: [] });
    await waitFor(() => expect(screen.queryByText("Thư ký gửi báo cáo")).not.toBeInTheDocument());
  });

  it("surfaces a failure toast when the override save errors", async () => {
    mockInvites([]);
    const memberA = makeMember({ full_name: "Giáo Viên A" });
    mockCenterMe(makeCenterMeOwner({ members: [selfMember(true), memberA] }));
    mockCenterPermissions(makeCenterPermissions({ members: [makeMemberPermissions(memberA)] }));
    server.use(
      http.put(`${API_URL}/centers/me/members/:teacherId/overrides`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "boom"), { status: 500 }),
      ),
    );
    renderCenter();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Phân quyền cho Giáo Viên A" }));
    const dialog = await screen.findByRole("dialog");
    await user.selectOptions(
      await within(dialog).findByRole("combobox", { name: "Quyền Gửi báo cáo học phí" }),
      "grant",
    );
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Có lỗi xảy ra, thử lại sau")).toBeInTheDocument();
  });
});

describe("CenterPage — regular member", () => {
  function mockMemberView() {
    return mockCenterMe(makeCenterMeMember({ center_name: "Trung Tâm Bình Minh" }));
  }

  it("shows only the center name and the member badge, no owner-only controls", async () => {
    mockMemberView();
    renderCenter();

    expect(await screen.findByText("Trung Tâm Bình Minh")).toBeInTheDocument();
    expect(screen.getByText("Thành viên")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Đổi tên trung tâm" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Xoá / })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /quyền gửi báo cáo/ })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Số điện thoại")).not.toBeInTheDocument();
  });
});

describe("CenterPage — degraded states", () => {
  it("shows the error state when the center cannot be loaded", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "boom"), { status: 500 }),
      ),
    );
    renderCenter();

    expect(await screen.findByText("Không tải được thông tin trung tâm.")).toBeInTheDocument();
  });
});
