import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/utils";

import { LoginPage } from "../pages/login-page";

function renderLogin(route = "/login") {
  return renderWithProviders(<LoginPage />, {
    route,
    path: "/login",
    extraRoutes: [
      { path: "/", element: <p>Dashboard home</p> },
      { path: "/classes", element: <p>Classes home</p> },
    ],
  });
}

describe("LoginPage", () => {
  it("shows required-field errors when submitted empty", async () => {
    const user = userEvent.setup();
    renderLogin();

    await user.click(screen.getByRole("button", { name: "Đăng nhập" }));

    expect(await screen.findByText("Vui lòng nhập số điện thoại")).toBeInTheDocument();
    expect(screen.getByText("Vui lòng nhập mật khẩu")).toBeInTheDocument();
  });

  it("shows a format error for a non-empty but invalid phone number", async () => {
    const user = userEvent.setup();
    renderLogin();

    await user.type(screen.getByLabelText("Số điện thoại"), "123");
    await user.type(screen.getByLabelText("Mật khẩu"), "some-password");
    await user.click(screen.getByRole("button", { name: "Đăng nhập" }));

    expect(await screen.findByText("Số điện thoại không hợp lệ")).toBeInTheDocument();
  });

  it("stores the session and navigates home on success", async () => {
    const user = userEvent.setup();
    renderLogin();

    await user.type(screen.getByLabelText("Số điện thoại"), "0901000001");
    await user.type(screen.getByLabelText("Mật khẩu"), "lan-password");
    await user.click(screen.getByRole("button", { name: "Đăng nhập" }));

    expect(await screen.findByText("Dashboard home")).toBeInTheDocument();
    expect(useAuthStore.getState().user?.phone).toBe("+84901000001");
    expect(useAuthStore.getState().accessToken).toBe("test-access-token");
  });

  it("returns to the originally requested page after login", async () => {
    const user = userEvent.setup();
    const { router } = renderLogin("/login");
    // Simulate arriving via ProtectedRoute's redirect state.
    await router.navigate("/login", { state: { from: "/classes" } });

    await user.type(screen.getByLabelText("Số điện thoại"), "0901000001");
    await user.type(screen.getByLabelText("Mật khẩu"), "lan-password");
    await user.click(screen.getByRole("button", { name: "Đăng nhập" }));

    expect(await screen.findByText("Classes home")).toBeInTheDocument();
  });

  it("shows the server message when credentials are rejected", async () => {
    server.use(
      http.post(`${API_URL}/auth/login`, () =>
        HttpResponse.json(fail("UNAUTHORIZED", "invalid phone or password"), { status: 401 }),
      ),
    );
    const user = userEvent.setup();
    renderLogin();

    await user.type(screen.getByLabelText("Số điện thoại"), "0901000001");
    await user.type(screen.getByLabelText("Mật khẩu"), "wrong-password");
    await user.click(screen.getByRole("button", { name: "Đăng nhập" }));

    expect(await screen.findByText("invalid phone or password")).toBeInTheDocument();
    expect(useAuthStore.getState().user).toBeNull();
  });
});
