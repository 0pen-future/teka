import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import { API_URL, fail } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/utils";

import { ResetPasswordPage } from "../pages/reset-password-page";

const PATH = "/reset-password/:token";

describe("ResetPasswordPage", () => {
  it("resets the password and redirects to login on success", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ResetPasswordPage />, {
      route: "/reset-password/good-token",
      path: PATH,
      extraRoutes: [{ path: "/login", element: <p>Trang đăng nhập</p> }],
    });

    await user.type(screen.getByLabelText("Mật khẩu mới"), "long-enough-password");
    await user.type(screen.getByLabelText("Xác nhận mật khẩu"), "long-enough-password");
    await user.click(screen.getByRole("button", { name: "Đặt lại mật khẩu" }));

    expect(await screen.findByText("Trang đăng nhập")).toBeInTheDocument();
  });

  it("shows a generic error on a rejected token, without branching on the reason", async () => {
    server.use(
      http.post(`${API_URL}/auth/reset-password`, () =>
        HttpResponse.json(fail("BAD_REQUEST", "invalid or expired token"), { status: 400 }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<ResetPasswordPage />, {
      route: "/reset-password/expired-token",
      path: PATH,
    });

    await user.type(screen.getByLabelText("Mật khẩu mới"), "long-enough-password");
    await user.type(screen.getByLabelText("Xác nhận mật khẩu"), "long-enough-password");
    await user.click(screen.getByRole("button", { name: "Đặt lại mật khẩu" }));

    expect(
      await screen.findByText(
        "Không thể đặt lại mật khẩu. Liên kết có thể đã hết hạn hoặc đã được dùng.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Đặt lại mật khẩu" })).toBeInTheDocument();
  });

  it("rejects a mismatched confirm password locally without calling the API", async () => {
    const user = userEvent.setup();
    renderWithProviders(<ResetPasswordPage />, {
      route: "/reset-password/good-token",
      path: PATH,
    });

    await user.type(screen.getByLabelText("Mật khẩu mới"), "long-enough-password");
    await user.type(screen.getByLabelText("Xác nhận mật khẩu"), "different-password");
    await user.click(screen.getByRole("button", { name: "Đặt lại mật khẩu" }));

    expect(await screen.findByText("Mật khẩu xác nhận không khớp")).toBeInTheDocument();
  });
});
