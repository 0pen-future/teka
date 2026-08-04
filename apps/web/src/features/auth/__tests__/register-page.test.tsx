import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/utils";

import { RegisterPage } from "../pages/register-page";

function renderRegister() {
  return renderWithProviders(<RegisterPage />, {
    route: "/register",
    path: "/register",
    extraRoutes: [{ path: "/", element: <p>Dashboard home</p> }],
  });
}

async function fillForm(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("Họ và tên"), "New Person");
  await user.type(screen.getByLabelText("Số điện thoại"), "0912345678");
  await user.type(screen.getByLabelText("Mật khẩu"), "long-enough-password");
}

describe("RegisterPage", () => {
  it("registers and lands on the dashboard", async () => {
    const user = userEvent.setup();
    renderRegister();

    await fillForm(user);
    await user.click(screen.getByRole("button", { name: "Tạo tài khoản" }));

    expect(await screen.findByText("Dashboard home")).toBeInTheDocument();
    expect(useAuthStore.getState().user?.phone).toBe("+84912345678");
  });

  it("pins a duplicate-phone conflict to the phone field", async () => {
    server.use(
      http.post(`${API_URL}/auth/register`, () =>
        HttpResponse.json(fail("CONFLICT", "phone already registered"), { status: 409 }),
      ),
    );
    const user = userEvent.setup();
    renderRegister();

    await fillForm(user);
    await user.click(screen.getByRole("button", { name: "Tạo tài khoản" }));

    const phoneInput = screen.getByLabelText("Số điện thoại");
    expect(await screen.findByText("phone already registered")).toBeInTheDocument();
    expect(phoneInput).toHaveAttribute("aria-invalid", "true");
    expect(useAuthStore.getState().user).toBeNull();
  });

  it("enforces the password length rule client-side", async () => {
    const user = userEvent.setup();
    renderRegister();

    await user.type(screen.getByLabelText("Họ và tên"), "New Person");
    await user.type(screen.getByLabelText("Số điện thoại"), "0912345678");
    await user.type(screen.getByLabelText("Mật khẩu"), "short");
    await user.click(screen.getByRole("button", { name: "Tạo tài khoản" }));

    expect(await screen.findByText("Mật khẩu tối thiểu 8 ký tự")).toBeInTheDocument();
  });
});
