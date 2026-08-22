import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { renderWithProviders } from "@/test/utils";

import { AcceptInvitePage } from "../pages/accept-invite-page";
import { mockAcceptInvite, mockInvitePreview } from "./invitation-handlers";

const PATH = "/invite/:token";

describe("AcceptInvitePage", () => {
  it("renders the account form once the preview resolves", async () => {
    mockInvitePreview("good-token", {
      center_name: "Trung Tâm Bình Minh",
      phone_masked: "+84******567",
    });
    renderWithProviders(<AcceptInvitePage />, { route: "/invite/good-token", path: PATH });

    expect(await screen.findByText("Trung Tâm Bình Minh")).toBeInTheDocument();
    expect(screen.getByText("Tạo tài khoản cho số +84******567")).toBeInTheDocument();
    expect(screen.getByLabelText("Họ và tên")).toBeInTheDocument();
  });

  it("shows the generic invite error when the preview fails", async () => {
    mockInvitePreview("good-token", null);
    renderWithProviders(<AcceptInvitePage />, { route: "/invite/expired-token", path: PATH });

    expect(await screen.findByText("Không mở được lời mời này.")).toBeInTheDocument();
  });

  it("creates the account and redirects to login on success", async () => {
    mockInvitePreview("good-token", {
      center_name: "Trung Tâm Bình Minh",
      phone_masked: "+84******567",
    });
    const captured = mockAcceptInvite(true);
    const user = userEvent.setup();
    renderWithProviders(<AcceptInvitePage />, {
      route: "/invite/good-token",
      path: PATH,
      extraRoutes: [{ path: "/login", element: <p>Trang đăng nhập</p> }],
    });

    await user.type(await screen.findByLabelText("Họ và tên"), "Cô Lan");
    await user.type(screen.getByLabelText("Mật khẩu"), "long-enough-password");
    await user.type(screen.getByLabelText("Xác nhận mật khẩu"), "long-enough-password");
    await user.click(screen.getByRole("button", { name: "Tạo tài khoản" }));

    expect(await screen.findByText("Trang đăng nhập")).toBeInTheDocument();
    expect(captured.received).toEqual({
      token: "good-token",
      full_name: "Cô Lan",
      password: "long-enough-password",
    });
  });

  it("shows one generic error on a rejected submission, without branching on the failure reason", async () => {
    mockInvitePreview("good-token", {
      center_name: "Trung Tâm Bình Minh",
      phone_masked: "+84******567",
    });
    mockAcceptInvite(false);
    const user = userEvent.setup();
    renderWithProviders(<AcceptInvitePage />, { route: "/invite/good-token", path: PATH });

    await user.type(await screen.findByLabelText("Họ và tên"), "Cô Lan");
    await user.type(screen.getByLabelText("Mật khẩu"), "long-enough-password");
    await user.type(screen.getByLabelText("Xác nhận mật khẩu"), "long-enough-password");
    await user.click(screen.getByRole("button", { name: "Tạo tài khoản" }));

    expect(
      await screen.findByText(
        "Không thể tạo tài khoản. Liên kết có thể đã hết hạn hoặc đã được dùng.",
      ),
    ).toBeInTheDocument();
  });
});
