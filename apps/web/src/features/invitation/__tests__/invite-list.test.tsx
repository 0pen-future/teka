import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import { API_URL, fail } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/utils";

import { InviteList } from "../components/invite-list";
import { makeInvite, mockInvites, mockRevokeInvite } from "./invitation-handlers";

describe("InviteList", () => {
  it("shows a loading state before the list resolves", () => {
    mockInvites([]);
    renderWithProviders(<InviteList />);

    expect(screen.getByText("Đang tải lời mời…")).toBeInTheDocument();
  });

  it("shows an empty state when there are no pending invites", async () => {
    mockInvites([makeInvite({ status: "accepted" })]);
    renderWithProviders(<InviteList />);

    expect(await screen.findByText("Chưa có lời mời nào đang chờ.")).toBeInTheDocument();
  });

  it("shows an error state when the list fails to load", async () => {
    server.use(
      http.get(`${API_URL}/centers/me/invitations`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "boom"), { status: 500 }),
      ),
    );
    renderWithProviders(<InviteList />);

    expect(await screen.findByText("Không tải được danh sách lời mời.")).toBeInTheDocument();
  });

  it("lists pending invites and revokes one", async () => {
    const invite = makeInvite({ phone: "+84901234567", status: "pending" });
    mockInvites([invite], []);
    const captured = mockRevokeInvite();
    const user = userEvent.setup();
    renderWithProviders(<InviteList />);

    const row = await screen.findByText("0901234567");
    expect(row).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: `Thu hồi lời mời 0901234567` }));

    expect(await screen.findByText("Chưa có lời mời nào đang chờ.")).toBeInTheDocument();
    expect(captured.revokedId).toBe(invite.id);
  });
});
