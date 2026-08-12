import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { CenterPage } from "../pages/center-page";
import { makeCenterMe, makeMember, mockCenterMe, mockJoinFailure } from "./center-handlers";

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

describe("CenterPage — owner of a shared center", () => {
  function mockSharedCenter() {
    return mockCenterMe(
      makeCenterMe({
        members: [selfMember(true), makeMember({ full_name: "Giáo Viên A" })],
      }),
    );
  }

  it("shows the center header with the owner badge and the member roster", async () => {
    mockSharedCenter();
    renderCenter();

    expect(await screen.findByText("Trung Tâm Bình Minh")).toBeInTheDocument();
    expect(screen.getByText("Chủ trung tâm")).toBeInTheDocument();
    expect(screen.getByText("Giáo Viên A")).toBeInTheDocument();
  });

  it("renames the center through the owner-only dialog", async () => {
    mockSharedCenter();
    let received: { name?: string } = {};
    server.use(
      http.patch(`${API_URL}/centers/me`, async ({ request }) => {
        received = (await request.json()) as typeof received;
        return HttpResponse.json(
          ok(
            makeCenterMe({
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

  it("removes a member after a confirm that says the data stays", async () => {
    // Post-refetch roster no longer contains the removed teacher.
    mockCenterMe(
      makeCenterMe({
        members: [selfMember(true), makeMember({ full_name: "Giáo Viên A" })],
      }),
      makeCenterMe({ members: [selfMember(true)] }),
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
    await user.click(screen.getByRole("button", { name: "Xoá khỏi trung tâm" }));

    expect(await screen.findByText("Đã xoá thành viên")).toBeInTheDocument();
    expect(deletedPath).not.toBe("");
    await waitFor(() => expect(screen.queryByText("Giáo Viên A")).not.toBeInTheDocument());
  });

  it("surfaces a failure toast when the removal errors", async () => {
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
    await user.click(await screen.findByRole("button", { name: "Xoá khỏi trung tâm" }));

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

  it("converges on 404 when the member already left — no red error", async () => {
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
    await user.click(await screen.findByRole("button", { name: "Xoá khỏi trung tâm" }));

    expect(await screen.findByText("Đã xoá thành viên")).toBeInTheDocument();
    expect(screen.queryByText("Something went wrong")).not.toBeInTheDocument();
  });

  it("never offers remove on the owner's own row, nor a leave button, nor the join section", async () => {
    mockSharedCenter();
    renderCenter();

    await screen.findByText("Giáo Viên A");
    expect(
      screen.queryByRole("button", { name: `Xoá ${testPrimaryTeacher.full_name}` }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Rời trung tâm" })).not.toBeInTheDocument();
    expect(screen.queryByText("Gia nhập trung tâm khác")).not.toBeInTheDocument();
  });
});

describe("CenterPage — owner alone in their personal center", () => {
  function mockPersonalCenter() {
    return mockCenterMe(makeCenterMe({ members: [selfMember(true)] }));
  }

  it("offers the join form and sends the owner phone", async () => {
    mockPersonalCenter();
    let received: { owner_phone?: string } = {};
    server.use(
      http.post(`${API_URL}/centers/join`, async ({ request }) => {
        received = (await request.json()) as typeof received;
        return HttpResponse.json(ok({ center_id: "c2", joined_at: "2026-08-12T08:00:00Z" }), {
          status: 201,
        });
      }),
    );
    const { queryClient } = renderCenter();
    // A stale scope-dependent cache entry: joining must evict it entirely —
    // mere invalidation would leave the old center's rows renderable.
    queryClient.setQueryData(["classes", "list"], { items: [] });
    const user = userEvent.setup();

    await user.type(await screen.findByLabelText("Số điện thoại chủ trung tâm"), "0901234567");
    await user.click(screen.getByRole("button", { name: "Gia nhập" }));

    expect(await screen.findByText("Đã gia nhập trung tâm")).toBeInTheDocument();
    expect(received).toEqual({ owner_phone: "0901234567" });
    expect(queryClient.getQueryState(["classes", "list"])).toBeUndefined();
  });

  it("rejects an invalid phone locally without calling the API", async () => {
    mockPersonalCenter();
    let calls = 0;
    server.use(
      http.post(`${API_URL}/centers/join`, () => {
        calls += 1;
        return HttpResponse.json(ok({ center_id: "c2", joined_at: "now" }), { status: 201 });
      }),
    );
    renderCenter();
    const user = userEvent.setup();

    await user.type(await screen.findByLabelText("Số điện thoại chủ trung tâm"), "12345");
    await user.click(screen.getByRole("button", { name: "Gia nhập" }));

    expect(await screen.findByText("Số điện thoại không hợp lệ")).toBeInTheDocument();
    expect(calls).toBe(0);
  });

  it.each([
    [404, "NOT_FOUND", "Không tìm thấy chủ trung tâm với số này"],
    [
      409,
      "CONFLICT",
      "Chưa thể gia nhập: tài khoản của bạn đã có dữ liệu hoặc thành viên. Vui lòng kiểm tra rồi thử lại.",
    ],
    [422, "VALIDATION_ERROR", "Không thể tự gia nhập trung tâm của chính mình"],
  ])("maps a %i join failure onto the form", async (status, code, expected) => {
    mockPersonalCenter();
    mockJoinFailure(status, code, "backend message");
    renderCenter();
    const user = userEvent.setup();

    await user.type(await screen.findByLabelText("Số điện thoại chủ trung tâm"), "0901234567");
    await user.click(screen.getByRole("button", { name: "Gia nhập" }));

    expect(await screen.findByText(expected)).toBeInTheDocument();
  });
});

describe("CenterPage — regular member", () => {
  function mockMemberView() {
    return mockCenterMe(
      makeCenterMe({
        center: { id: "c1", name: "Trung Tâm Bình Minh", is_owner: false },
        members: [makeMember({ full_name: "Cô Chủ", is_owner: true }), selfMember(false)],
      }),
    );
  }

  it("hides every owner-only control and shows the member badge", async () => {
    mockMemberView();
    renderCenter();

    expect(await screen.findByText("Trung Tâm Bình Minh")).toBeInTheDocument();
    expect(screen.getByText("Thành viên")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Đổi tên trung tâm" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Xoá / })).not.toBeInTheDocument();
    expect(screen.queryByText("Gia nhập trung tâm khác")).not.toBeInTheDocument();
  });

  it("leaves the center after a confirm that says created data stays behind", async () => {
    mockMemberView();
    let deletedPath = "";
    server.use(
      http.delete(`${API_URL}/centers/me/members/:teacherId`, ({ params }) => {
        deletedPath = String(params.teacherId);
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { queryClient } = renderCenter();
    queryClient.setQueryData(["classes", "list"], { items: [] });
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Rời trung tâm" }));
    expect(await screen.findByText(/ở lại trung tâm/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Rời khỏi trung tâm" }));

    expect(await screen.findByText("Bạn đã rời trung tâm")).toBeInTheDocument();
    expect(deletedPath).toBe(testPrimaryTeacher.id);
    expect(queryClient.getQueryState(["classes", "list"])).toBeUndefined();
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
