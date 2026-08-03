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
      { path: "/users", element: <p>Users home</p> },
    ],
  });
}

describe("LoginPage", () => {
  it("shows validation errors without calling the API", async () => {
    const user = userEvent.setup();
    renderLogin();

    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByText("Enter a valid email address")).toBeInTheDocument();
    expect(screen.getByText("Password is required")).toBeInTheDocument();
  });

  it("stores the session and navigates home on success", async () => {
    const user = userEvent.setup();
    renderLogin();

    await user.type(screen.getByLabelText("Email"), "admin@example.com");
    await user.type(screen.getByLabelText("Password"), "correct-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByText("Dashboard home")).toBeInTheDocument();
    expect(useAuthStore.getState().user?.email).toBe("admin@example.com");
    expect(useAuthStore.getState().accessToken).toBe("test-access-token");
  });

  it("returns to the originally requested page after login", async () => {
    const user = userEvent.setup();
    const { router } = renderLogin("/login");
    // Simulate arriving via ProtectedRoute's redirect state.
    await router.navigate("/login", { state: { from: "/users" } });

    await user.type(screen.getByLabelText("Email"), "admin@example.com");
    await user.type(screen.getByLabelText("Password"), "correct-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByText("Users home")).toBeInTheDocument();
  });

  it("shows the server message when credentials are rejected", async () => {
    server.use(
      http.post(`${API_URL}/auth/login`, () =>
        HttpResponse.json(fail("UNAUTHORIZED", "invalid email or password"), { status: 401 }),
      ),
    );
    const user = userEvent.setup();
    renderLogin();

    await user.type(screen.getByLabelText("Email"), "admin@example.com");
    await user.type(screen.getByLabelText("Password"), "wrong-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByText("invalid email or password")).toBeInTheDocument();
    expect(useAuthStore.getState().user).toBeNull();
  });
});
