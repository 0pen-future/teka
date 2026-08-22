import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { mockZaloStatus } from "@/features/profile/__tests__/zalo-handlers";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ContactDetailPage } from "../pages/contact-detail-page";
import { ContactsPage } from "../pages/contacts-page";
import {
  contactSingleChild,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
} from "./roster-handlers";

interface FriendFixture {
  user_id: string;
  display_name: string;
  avatar?: string;
}

const friendMeLan: FriendFixture = {
  user_id: "zl-user-001",
  display_name: "Mẹ Lan",
  avatar: "https://example.com/me-lan.png",
};

const friendBoHung: FriendFixture = { user_id: "zl-user-002", display_name: "Bố Hùng" };
const friendDiDao: FriendFixture = { user_id: "zl-user-003", display_name: "Dì Đào" };

/** Serves `GET /me/zalo/friends`, counting calls so tests can assert "never fetched". */
function mockZaloFriends(friends: FriendFixture[]) {
  const calls = { count: 0 };
  server.use(
    http.get(`${API_URL}/me/zalo/friends`, () => {
      calls.count += 1;
      return HttpResponse.json(ok(friends));
    }),
  );
  return calls;
}

function renderContactDetail() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ContactDetailPage />, {
    route: `/contacts/${contactSingleChild.id}`,
    path: "/contacts/:id",
    extraRoutes: [{ path: "/profile", element: <div>Trang cá nhân</div> }],
  });
}

/** The stored contact the detail page renders, pre-mapped to a Zalo friend. */
function mapStoredContact(friend: FriendFixture) {
  const contact = getRosterStore().contacts.find((item) => item.id === contactSingleChild.id);
  if (!contact) {
    throw new Error("fixture contact missing from store");
  }
  contact.zalo_user_id = friend.user_id;
  contact.zalo_name = friend.display_name;
  return contact;
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("Zalo card on the contact detail page", () => {
  it("points to the profile page and never fetches friends while Zalo is unlinked", async () => {
    // The shared default `GET /me/zalo` already answers "not linked".
    const friendCalls = mockZaloFriends([friendMeLan]);
    renderContactDetail();

    const link = await screen.findByRole("link", { name: "Kết nối Zalo trước" });
    expect(link).toHaveAttribute("href", "/profile");
    expect(screen.queryByRole("button", { name: "Chọn bạn Zalo" })).not.toBeInTheDocument();
    expect(friendCalls.count).toBe(0);
  });

  it("asks for a re-scan when the stored session expired", async () => {
    mockZaloStatus({ linked: true, status: "expired", display_name: "Cô Lan" });
    const friendCalls = mockZaloFriends([friendMeLan]);
    renderContactDetail();

    const link = await screen.findByRole("link", { name: "Quét lại mã" });
    expect(link).toHaveAttribute("href", "/profile");
    expect(screen.queryByRole("button", { name: "Chọn bạn Zalo" })).not.toBeInTheDocument();
    expect(friendCalls.count).toBe(0);
  });

  it("maps the contact to a friend picked from the modal", async () => {
    mockZaloStatus({ linked: true, status: "linked", display_name: "Cô Lan" });
    mockZaloFriends([friendMeLan, friendBoHung, friendDiDao]);
    const user = userEvent.setup();
    renderContactDetail();

    await user.click(await screen.findByRole("button", { name: "Chọn bạn Zalo" }));
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByRole("option", { name: /Mẹ Lan/ });

    await user.click(within(dialog).getByRole("option", { name: /Mẹ Lan/ }));

    // Card re-renders from the invalidated contact query — no reload involved.
    await screen.findByText("Mẹ Lan");
    expect(screen.getByRole("button", { name: "Bỏ liên kết" })).toBeInTheDocument();
    const stored = getRosterStore().contacts.find((item) => item.id === contactSingleChild.id);
    expect(stored?.zalo_user_id).toBe(friendMeLan.user_id);
    expect(stored?.zalo_name).toBe("Mẹ Lan");
  });

  it("filters friends by name, ignoring Vietnamese diacritics", async () => {
    mockZaloStatus({ linked: true, status: "linked", display_name: "Cô Lan" });
    mockZaloFriends([friendMeLan, friendBoHung, friendDiDao]);
    const user = userEvent.setup();
    renderContactDetail();

    await user.click(await screen.findByRole("button", { name: "Chọn bạn Zalo" }));
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByRole("option", { name: /Bố Hùng/ });

    await user.type(within(dialog).getByPlaceholderText("Tìm theo tên"), "dao");

    expect(within(dialog).getByRole("option", { name: /Dì Đào/ })).toBeInTheDocument();
    expect(within(dialog).queryByRole("option", { name: /Mẹ Lan/ })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("option", { name: /Bố Hùng/ })).not.toBeInTheDocument();
  });

  it("keeps the picker open and explains when the friend is already mapped elsewhere", async () => {
    mockZaloStatus({ linked: true, status: "linked", display_name: "Cô Lan" });
    mockZaloFriends([friendMeLan]);
    const other = getRosterStore().contacts.find((item) => item.id !== contactSingleChild.id);
    if (other) {
      other.zalo_user_id = friendMeLan.user_id;
      other.zalo_name = friendMeLan.display_name;
    }
    const user = userEvent.setup();
    renderContactDetail();

    await user.click(await screen.findByRole("button", { name: "Chọn bạn Zalo" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(await within(dialog).findByRole("option", { name: /Mẹ Lan/ }));

    await within(dialog).findByText("Bạn này đã được liên kết với người liên hệ khác.");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("recovers from a failed friend list with a retry", async () => {
    mockZaloStatus({ linked: true, status: "linked", display_name: "Cô Lan" });
    let failNext = true;
    server.use(
      http.get(`${API_URL}/me/zalo/friends`, () => {
        if (failNext) {
          failNext = false;
          return HttpResponse.json(
            { success: false, error: { code: "BAD_GATEWAY", message: "zalo unreachable" } },
            { status: 502 },
          );
        }
        return HttpResponse.json(ok([friendMeLan]));
      }),
    );
    const user = userEvent.setup();
    renderContactDetail();

    await user.click(await screen.findByRole("button", { name: "Chọn bạn Zalo" }));
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText("Không tải được danh sách bạn bè.");

    await user.click(within(dialog).getByRole("button", { name: "Thử lại" }));

    await within(dialog).findByRole("option", { name: /Mẹ Lan/ });
  });

  it("unmaps the contact after an explicit confirmation", async () => {
    mockZaloStatus({ linked: true, status: "linked", display_name: "Cô Lan" });
    mapStoredContact(friendMeLan);
    const user = userEvent.setup();
    renderContactDetail();

    await user.click(await screen.findByRole("button", { name: "Bỏ liên kết" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Bỏ liên kết" }));

    await screen.findByRole("button", { name: "Chọn bạn Zalo" });
    await waitFor(() => {
      const stored = getRosterStore().contacts.find((item) => item.id === contactSingleChild.id);
      expect(stored?.zalo_user_id).toBeUndefined();
    });
  });

  it("surfaces a failed unmap instead of failing silently", async () => {
    mockZaloStatus({ linked: true, status: "linked", display_name: "Cô Lan" });
    mapStoredContact(friendMeLan);
    server.use(
      http.delete(`${API_URL}/contacts/:id/zalo-mapping`, () =>
        HttpResponse.json(
          { success: false, error: { code: "INTERNAL_ERROR", message: "boom" } },
          { status: 500 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderContactDetail();

    await user.click(await screen.findByRole("button", { name: "Bỏ liên kết" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Bỏ liên kết" }));

    await screen.findByText("Không thể bỏ liên kết. Thử lại sau.");
    // The mapping survives an unmap that never reached the server.
    expect(
      getRosterStore().contacts.find((item) => item.id === contactSingleChild.id)?.zalo_user_id,
    ).toBe(friendMeLan.user_id);
  });

  it("does not present a failed status read as 'not linked'", async () => {
    let failStatus = true;
    server.use(
      http.get(`${API_URL}/me/zalo`, () => {
        if (failStatus) {
          failStatus = false;
          return HttpResponse.json(
            { success: false, error: { code: "INTERNAL_ERROR", message: "boom" } },
            { status: 500 },
          );
        }
        return HttpResponse.json(ok({ linked: true, status: "linked", display_name: "Cô Lan" }));
      }),
    );
    const user = userEvent.setup();
    renderContactDetail();

    await screen.findByText("Không tải được trạng thái Zalo.");
    expect(screen.queryByRole("link", { name: "Kết nối Zalo trước" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Thử lại" }));

    await screen.findByRole("button", { name: "Chọn bạn Zalo" });
  });

  it("routes to the profile page when the session died between status read and picker open", async () => {
    mockZaloStatus({ linked: true, status: "linked", display_name: "Cô Lan" });
    server.use(
      http.get(`${API_URL}/me/zalo/friends`, () =>
        HttpResponse.json(
          { success: false, error: { code: "CONFLICT", message: "zalo session expired" } },
          { status: 409 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderContactDetail();

    await user.click(await screen.findByRole("button", { name: "Chọn bạn Zalo" }));
    const dialog = await screen.findByRole("dialog");

    await within(dialog).findByText(/Phiên Zalo không còn hiệu lực/);
    const link = within(dialog).getByRole("link", { name: "Quét lại mã" });
    expect(link).toHaveAttribute("href", "/profile");
    expect(within(dialog).queryByRole("button", { name: "Thử lại" })).not.toBeInTheDocument();
  });
});

describe("mapped badge on the contacts list", () => {
  it("marks mapped contacts with the stored Zalo name", async () => {
    mapStoredContact(friendMeLan);
    signInAs(testPrimaryTeacher);
    renderWithProviders(<ContactsPage />, { route: "/contacts", path: "/contacts" });

    const mappedRow = (await screen.findByText("Nguyễn Thị Lan")).closest("a");
    const otherRow = screen.getByText("Phạm Văn Hùng").closest("a");
    expect(mappedRow).not.toBeNull();
    expect(otherRow).not.toBeNull();
    expect(within(mappedRow as HTMLElement).getByText("Mẹ Lan")).toBeInTheDocument();
    expect(within(otherRow as HTMLElement).queryByText("Mẹ Lan")).not.toBeInTheDocument();
  });
});
