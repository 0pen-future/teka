import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { renderWithProviders } from "@/test/utils";

import { InviteSection } from "../components/invite-section";
import { makeCreateInviteResponse, mockCreateInvite, mockInvites } from "./invitation-handlers";

describe("InviteSection", () => {
  it("creates an invite and shows the copyable link dialog", async () => {
    mockInvites([]);
    const captured = mockCreateInvite(
      makeCreateInviteResponse({ phone: "+84901234567", dm_status: "sent" }),
    );
    const user = userEvent.setup();
    renderWithProviders(<InviteSection />);

    await user.type(await screen.findByLabelText("Số điện thoại"), "0901234567");
    await user.click(screen.getByRole("button", { name: "Gửi lời mời" }));

    expect(await screen.findByText("Đã tạo lời mời")).toBeInTheDocument();
    expect(captured.received).toEqual({ phone: "0901234567" });
    expect(screen.getByText("Đã gửi qua Zalo")).toBeInTheDocument();
    expect(screen.getByLabelText("Liên kết mời")).toHaveValue(
      "https://app.teka.dev/invite/test-invite-token",
    );
  });

  it("copies the link to the clipboard", async () => {
    mockInvites([]);
    mockCreateInvite(makeCreateInviteResponse());
    // userEvent.setup() installs its own Clipboard stub on navigator.clipboard
    // (replacing anything defined beforehand), so the spy must attach after
    // setup() runs, not in a beforeEach.
    const user = userEvent.setup();
    const writeText = vi.spyOn(navigator.clipboard, "writeText");
    renderWithProviders(<InviteSection />);

    await user.type(await screen.findByLabelText("Số điện thoại"), "0901234567");
    await user.click(screen.getByRole("button", { name: "Gửi lời mời" }));
    await screen.findByText("Đã tạo lời mời");
    await user.click(screen.getByRole("button", { name: "Sao chép liên kết" }));

    expect(await screen.findByRole("button", { name: "Đã sao chép liên kết" })).toBeInTheDocument();
    expect(writeText).toHaveBeenCalledWith("https://app.teka.dev/invite/test-invite-token");
  });

  it.each([
    ["sent", "Đã gửi qua Zalo"],
    ["skipped", "Chưa gửi qua Zalo"],
    ["failed", "Gửi Zalo thất bại"],
  ] as const)("shows the %s DM status badge", async (dmStatus, label) => {
    mockInvites([]);
    mockCreateInvite(makeCreateInviteResponse({ dm_status: dmStatus }));
    const user = userEvent.setup();
    renderWithProviders(<InviteSection />);

    await user.type(await screen.findByLabelText("Số điện thoại"), "0901234567");
    await user.click(screen.getByRole("button", { name: "Gửi lời mời" }));

    expect(await screen.findByText(label)).toBeInTheDocument();
  });

  it("rejects an invalid phone locally without calling the API", async () => {
    mockInvites([]);
    const captured = mockCreateInvite(makeCreateInviteResponse());
    const user = userEvent.setup();
    renderWithProviders(<InviteSection />);

    await user.type(await screen.findByLabelText("Số điện thoại"), "12345");
    await user.click(screen.getByRole("button", { name: "Gửi lời mời" }));

    expect(await screen.findByText("Số điện thoại không hợp lệ")).toBeInTheDocument();
    expect(captured.received).toEqual({});
  });
});
