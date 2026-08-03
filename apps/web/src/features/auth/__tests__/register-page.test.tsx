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
  await user.type(screen.getByLabelText("Name"), "New Person");
  await user.type(screen.getByLabelText("Email"), "new@example.com");
  await user.type(screen.getByLabelText("Password"), "long-enough-password");
}

describe("RegisterPage", () => {
  it("registers and lands on the dashboard", async () => {
    const user = userEvent.setup();
    renderRegister();

    await fillForm(user);
    await user.click(screen.getByRole("button", { name: "Create account" }));

    expect(await screen.findByText("Dashboard home")).toBeInTheDocument();
    expect(useAuthStore.getState().user?.email).toBe("new@example.com");
  });

  it("pins a duplicate-email conflict to the email field", async () => {
    server.use(
      http.post(`${API_URL}/auth/register`, () =>
        HttpResponse.json(fail("CONFLICT", "email already in use"), { status: 409 }),
      ),
    );
    const user = userEvent.setup();
    renderRegister();

    await fillForm(user);
    await user.click(screen.getByRole("button", { name: "Create account" }));

    const emailInput = screen.getByLabelText("Email");
    expect(await screen.findByText("email already in use")).toBeInTheDocument();
    expect(emailInput).toHaveAttribute("aria-invalid", "true");
    expect(useAuthStore.getState().user).toBeNull();
  });

  it("enforces the password length rule client-side", async () => {
    const user = userEvent.setup();
    renderRegister();

    await user.type(screen.getByLabelText("Name"), "New Person");
    await user.type(screen.getByLabelText("Email"), "new@example.com");
    await user.type(screen.getByLabelText("Password"), "short");
    await user.click(screen.getByRole("button", { name: "Create account" }));

    expect(await screen.findByText("Password must be at least 8 characters")).toBeInTheDocument();
  });
});
