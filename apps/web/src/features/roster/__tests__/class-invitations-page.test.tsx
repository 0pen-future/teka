import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import {
  renderWithProviders,
  signInAs,
  testPrimaryTeacher,
  testSecondaryTeacher,
} from "@/test/utils";
import { mockViewport } from "@/test/viewport";

import { ClassInvitationsPage } from "../pages/class-invitations-page";
import {
  classWithSchedule,
  getRosterStore,
  invitationAcceptedGiaoVien,
  invitationPendingTroGiang,
  resetRosterStore,
  rosterHandlers,
} from "./roster-handlers";

/** Member body: the invitee's own view — no owner actions anywhere. */
const memberCenterHandler = http.get(`${API_URL}/centers/me`, () =>
  HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions: [] })),
);

function rowOf(name: string) {
  return screen.getByRole("row", { name: new RegExp(name) });
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
  mockViewport(1024);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("ClassInvitationsPage as the invited member", () => {
  beforeEach(() => {
    server.use(memberCenterHandler);
    signInAs(testSecondaryTeacher);
  });

  it("lists every invitation with class, role and status, answering only the pending one", async () => {
    renderWithProviders(<ClassInvitationsPage />);

    expect(await screen.findByRole("heading", { name: "Lời mời nhận lớp" })).toBeInTheDocument();
    const pending = await screen.findByRole("row", { name: /Cô Hương/ });
    expect(within(pending).getByText("Toán 6A")).toBeInTheDocument();
    expect(within(pending).getByText("Trợ giảng")).toBeInTheDocument();
    expect(within(pending).getByText("Chờ xác nhận")).toBeInTheDocument();
    expect(within(pending).getByText("Chưa gắn khóa")).toBeInTheDocument();
    expect(within(pending).getByRole("button", { name: "Chấp nhận" })).toBeInTheDocument();
    expect(within(pending).getByRole("button", { name: "Từ chối" })).toBeInTheDocument();

    const accepted = rowOf("Thầy Nam");
    expect(within(accepted).getByText("Đã đồng ý")).toBeInTheDocument();
    expect(within(accepted).queryByRole("button")).not.toBeInTheDocument();

    expect(screen.queryByRole("button", { name: "Nhắc lại" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "GV nhận lớp" })).not.toBeInTheDocument();
  });

  it("accepting moves the row to Đã đồng ý and toasts", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ClassInvitationsPage />);
    const pending = await screen.findByRole("row", { name: /Cô Hương/ });

    await user.click(within(pending).getByRole("button", { name: "Chấp nhận" }));

    expect(await screen.findByText("Đã nhận lời mời lớp Toán 6A")).toBeInTheDocument();
    await waitFor(() => {
      expect(within(rowOf("Cô Hương")).getByText("Đã đồng ý")).toBeInTheDocument();
    });
    expect(
      getRosterStore().classInvitations.find((item) => item.id === invitationPendingTroGiang.id)
        ?.status,
    ).toBe("accepted");
  });

  it("declining marks the row Đã từ chối", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ClassInvitationsPage />);
    const pending = await screen.findByRole("row", { name: /Cô Hương/ });

    await user.click(within(pending).getByRole("button", { name: "Từ chối" }));

    await waitFor(() => {
      expect(within(rowOf("Cô Hương")).getByText("Đã từ chối")).toBeInTheDocument();
    });
    expect(
      getRosterStore().classInvitations.find((item) => item.id === invitationPendingTroGiang.id)
        ?.status,
    ).toBe("declined");
  });

  it("status chips filter the table client-side and carry counts", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ClassInvitationsPage />);
    await screen.findByRole("row", { name: /Cô Hương/ });

    const chips = screen.getByRole("radiogroup", { name: "Lọc theo trạng thái" });
    expect(within(chips).getByRole("radio", { name: /Tất cả/ })).toHaveTextContent("3");
    expect(within(chips).getByRole("radio", { name: /Đã nhận/ })).toHaveTextContent("1");

    await user.click(within(chips).getByRole("radio", { name: /Từ chối/ }));

    expect(screen.getAllByRole("row")).toHaveLength(2); // header + one declined row
    expect(screen.getByText("Học vụ")).toBeInTheDocument();
    expect(screen.queryByText("Trợ giảng")).not.toBeInTheDocument();
  });

  it("renders the prototype layout: subtitle, numbered columns and the class's course", async () => {
    getRosterStore().classInvitations[0] = {
      ...getRosterStore().classInvitations[0]!,
      course_name: "Toán 6 nâng cao",
    };
    renderWithProviders(<ClassInvitationsPage />);

    expect(
      await screen.findByText(
        "Giáo viên được mời vào lớp phải xác nhận trước khi xuất hiện trong Đội ngũ giảng dạy.",
      ),
    ).toBeInTheDocument();
    const first = await screen.findByRole("row", { name: /Cô Hương/ });
    const headers = screen.getAllByRole("columnheader").map((cell) => cell.textContent);
    expect(headers).toEqual(["STT", "Lớp học", "Giáo viên", "Gửi lúc", "Trạng thái", "Thao tác"]);
    expect(within(first).getByText("1")).toBeInTheDocument();
    expect(within(first).getByText("Toán 6 nâng cao")).toBeInTheDocument();
  });

  it("offers only the prototype's four chips and keeps cancelled rows under Tất cả", async () => {
    const store = getRosterStore();
    store.classInvitations[2] = { ...store.classInvitations[2]!, status: "cancelled" };
    const user = userEvent.setup();
    renderWithProviders(<ClassInvitationsPage />);
    await screen.findByRole("row", { name: /Cô Hoa/ });

    const chips = screen.getByRole("radiogroup", { name: "Lọc theo trạng thái" });
    expect(
      within(chips)
        .getAllByRole("radio")
        .map((chip) => chip.firstChild?.textContent),
    ).toEqual(["Tất cả", "Chờ xác nhận", "Đã nhận", "Từ chối"]);
    expect(within(rowOf("Cô Hoa")).getByText("Đã hủy")).toBeInTheDocument();

    await user.click(within(chips).getByRole("radio", { name: /Từ chối/ }));
    expect(await screen.findByText("Không có lời mời nào.")).toBeInTheDocument();
  });

  it("shows the empty state when nothing was ever sent", async () => {
    getRosterStore().classInvitations = [];
    renderWithProviders(<ClassInvitationsPage />);

    expect(await screen.findByText("Không có lời mời nào.")).toBeInTheDocument();
  });
});

describe("ClassInvitationsPage as the owner", () => {
  beforeEach(() => {
    signInAs(testPrimaryTeacher);
  });

  it("offers remind and cancel on a pending row, never the invitee's answer buttons", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ClassInvitationsPage />);
    const pending = await screen.findByRole("row", { name: /Cô Hương/ });

    expect(within(pending).queryByRole("button", { name: "Chấp nhận" })).not.toBeInTheDocument();
    await user.click(within(pending).getByRole("button", { name: "Nhắc lại" }));
    expect(await screen.findByText("Đã nhắc lại Cô Hương")).toBeInTheDocument();
    expect(
      getRosterStore().classInvitations.find((item) => item.id === invitationPendingTroGiang.id)
        ?.reminded_at,
    ).not.toBeNull();

    await user.click(within(rowOf("Cô Hương")).getByRole("button", { name: "Hủy" }));
    const dialog = await screen.findByRole("dialog", { name: /Hủy lời mời/ });
    await user.click(within(dialog).getByRole("button", { name: "Hủy lời mời" }));

    await waitFor(() => {
      expect(within(rowOf("Cô Hương")).getByText("Đã hủy")).toBeInTheDocument();
    });
  });

  it("orders owner actions as in the prototype: nhận lớp, nhắc lại, hủy", async () => {
    renderWithProviders(<ClassInvitationsPage />);
    const pending = await screen.findByRole("row", { name: /Cô Hương/ });

    expect(
      within(pending)
        .getAllByRole("button")
        .map((button) => button.textContent),
    ).toEqual(["Phân công", "Nhắc lại", "Hủy"]);
    // Once the invitee said yes there is nothing left to remind.
    expect(
      within(rowOf("Thầy Nam"))
        .getAllByRole("button")
        .map((button) => button.textContent),
    ).toEqual(["GV nhận lớp", "Hủy"]);
    // Finished rows carry no actions.
    expect(within(rowOf("Cô Hoa")).queryByRole("button")).not.toBeInTheDocument();
  });

  it("confirming an accepted giao_vien invite names the replaced teacher and hands the class over", async () => {
    const user = userEvent.setup();
    const { queryClient } = renderWithProviders(<ClassInvitationsPage />);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const accepted = await screen.findByRole("row", { name: /Thầy Nam/ });

    await user.click(within(accepted).getByRole("button", { name: "GV nhận lớp" }));

    const dialog = await screen.findByRole("dialog", { name: /Thầy Nam nhận lớp Toán 6A/ });
    expect(await within(dialog).findByText(/thay Cô Lan/)).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Xác nhận" }));

    expect(
      await screen.findByText("Thầy Nam đã nhận lớp Toán 6A · chuyển 2 buổi sắp tới"),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(within(rowOf("Thầy Nam")).getByText("Đã nhận lớp")).toBeInTheDocument();
    });
    const store = getRosterStore();
    expect(
      store.classInvitations.find((item) => item.id === invitationAcceptedGiaoVien.id)?.status,
    ).toBe("assigned");
    expect(store.classes.find((item) => item.id === classWithSchedule.id)?.teacher_id).toBe(
      invitationAcceptedGiaoVien.teacher_id,
    );
    expect(
      store.classStaff.filter((item) => item.role_key === "giao_vien" && item.ended_at === null),
    ).toHaveLength(1);
    // A handoff repoints the class and moves its upcoming sessions, so every
    // class and session query refetches, not just this class's detail.
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["roster", "classes"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["attendance", "sessions"] });
  });

  it("keeps Xác nhận off until the current teacher is known for a handoff", async () => {
    const user = userEvent.setup();
    server.use(
      http.get(
        `${API_URL}/classes/:classId/staff`,
        () => HttpResponse.json({ error: { code: "INTERNAL", message: "boom" } }, { status: 500 }),
        { once: true },
      ),
    );
    renderWithProviders(<ClassInvitationsPage />);
    const accepted = await screen.findByRole("row", { name: /Thầy Nam/ });

    await user.click(within(accepted).getByRole("button", { name: "GV nhận lớp" }));
    const dialog = await screen.findByRole("dialog", { name: /Thầy Nam nhận lớp Toán 6A/ });
    expect(
      await within(dialog).findByText(/Không tải được giáo viên hiện tại của lớp/),
    ).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Xác nhận" })).toBeDisabled();

    // Once the staff list loads, the handoff names the replaced teacher and unlocks.
    await user.click(within(dialog).getByRole("button", { name: "Thử lại" }));
    expect(await within(dialog).findByText(/thay Cô Lan/)).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Xác nhận" })).toBeEnabled();
  });

  it("surfaces the API's conflict message when the confirm is refused", async () => {
    const user = userEvent.setup();
    server.use(
      http.post(`${API_URL}/class-invitations/:id/confirm`, () =>
        HttpResponse.json(
          { error: { code: "MEMBER_INACTIVE", message: "thành viên đã rời trung tâm" } },
          { status: 422 },
        ),
      ),
    );
    renderWithProviders(<ClassInvitationsPage />);
    const accepted = await screen.findByRole("row", { name: /Thầy Nam/ });

    await user.click(within(accepted).getByRole("button", { name: "GV nhận lớp" }));
    const dialog = await screen.findByRole("dialog", { name: /Thầy Nam nhận lớp Toán 6A/ });
    await user.click(within(dialog).getByRole("button", { name: "Xác nhận" }));

    expect(await within(dialog).findByText("thành viên đã rời trung tâm")).toBeInTheDocument();
    // The dialog stays open; closing it reveals the row still waiting for a confirm.
    await user.click(within(dialog).getByRole("button", { name: "Huỷ" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(within(rowOf("Thầy Nam")).getByText("Đã đồng ý")).toBeInTheDocument();
  });
});
