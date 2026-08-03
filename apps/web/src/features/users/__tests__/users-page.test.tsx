import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import {
  aliceUser,
  API_URL,
  defaultUsers,
  fail,
  listMeta,
  makeUser,
  ok,
} from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testAdmin, testUser } from "@/test/utils";

import { UsersPage } from "../pages/users-page";

function renderUsers(route = "/users") {
  return renderWithProviders(<UsersPage />, { route, path: "/users" });
}

describe("UsersPage", () => {
  it("shows skeleton rows while the list loads", () => {
    signInAs(testAdmin);
    const { container } = renderUsers();

    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0);
  });

  it("renders the user rows and pagination summary", async () => {
    signInAs(testAdmin);
    renderUsers();

    expect(await screen.findByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("admin@example.com")).toBeInTheDocument();
  });

  it("shows the empty state when no users match", async () => {
    server.use(http.get(`${API_URL}/users`, () => HttpResponse.json(ok([], listMeta(0)))));
    signInAs(testAdmin);
    renderUsers();

    expect(await screen.findByText("No users found")).toBeInTheDocument();
  });

  it("shows an error state and recovers via retry", async () => {
    server.use(
      http.get(`${API_URL}/users`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "something went wrong"), { status: 500 }),
      ),
    );
    signInAs(testAdmin);
    renderUsers();

    expect(await screen.findByText("Could not load users")).toBeInTheDocument();
    expect(screen.getByText("something went wrong")).toBeInTheDocument();

    server.resetHandlers();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("Alice")).toBeInTheDocument();
  });

  it("debounces search into the URL and the API query", async () => {
    let lastListUrl: URL | null = null;
    server.use(
      http.get(`${API_URL}/users`, ({ request }) => {
        lastListUrl = new URL(request.url);
        return HttpResponse.json(ok([aliceUser], listMeta(1)));
      }),
    );
    signInAs(testAdmin);
    const { router } = renderUsers();
    const user = userEvent.setup();

    await user.type(screen.getByLabelText("Search users"), "alice");

    await waitFor(() => {
      expect(router.state.location.search).toContain("q=alice");
    });
    await waitFor(() => {
      expect(lastListUrl?.searchParams.get("q")).toBe("alice");
    });
  });

  it("pages through results via the URL", async () => {
    const manyUsers = Array.from({ length: 5 }, () => makeUser());
    server.use(
      http.get(`${API_URL}/users`, ({ request }) => {
        const page = Number(new URL(request.url).searchParams.get("page") ?? "1");
        return HttpResponse.json(ok(manyUsers, { page, per_page: 20, total: 40, total_pages: 2 }));
      }),
    );
    signInAs(testAdmin);
    const { router } = renderUsers();

    expect(await screen.findByText("Page 1 of 2 · 40 total")).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Next" }));

    await waitFor(() => {
      expect(router.state.location.search).toContain("page=2");
    });
    expect(await screen.findByText("Page 2 of 2 · 40 total")).toBeInTheDocument();
  });

  it("hides admin actions from non-admin users", async () => {
    signInAs(testUser);
    renderUsers();

    expect(await screen.findByText("Alice")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "New user" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Actions for/ })).not.toBeInTheDocument();
  });

  it("pins a duplicate-email conflict to the email field when creating", async () => {
    server.use(
      http.post(`${API_URL}/users`, () =>
        HttpResponse.json(fail("CONFLICT", "email already in use"), { status: 409 }),
      ),
    );
    signInAs(testAdmin);
    renderUsers();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "New user" }));
    await user.type(screen.getByLabelText("Name"), "Duplicate Person");
    await user.type(screen.getByLabelText("Email"), "alice@example.com");
    await user.type(screen.getByLabelText("Password"), "long-enough-password");
    await user.click(screen.getByRole("button", { name: "Create user" }));

    expect(await screen.findByText("email already in use")).toBeInTheDocument();
    expect(screen.getByLabelText("Email")).toHaveAttribute("aria-invalid", "true");
  });

  it("maps server validation fields onto the create form", async () => {
    server.use(
      http.post(`${API_URL}/users`, () =>
        HttpResponse.json(
          fail("VALIDATION_ERROR", "validation failed", { password: "is too weak" }),
          { status: 422 },
        ),
      ),
    );
    signInAs(testAdmin);
    renderUsers();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "New user" }));
    await user.type(screen.getByLabelText("Name"), "New Person");
    await user.type(screen.getByLabelText("Email"), "new@example.com");
    await user.type(screen.getByLabelText("Password"), "long-enough-password");
    await user.click(screen.getByRole("button", { name: "Create user" }));

    expect(await screen.findByText("is too weak")).toBeInTheDocument();
  });

  it("deletes a user after confirmation", async () => {
    let deletedId: string | null = null;
    server.use(
      http.delete(`${API_URL}/users/:id`, ({ params }) => {
        deletedId = String(params.id);
        return HttpResponse.json(ok({ message: "user deleted" }));
      }),
      http.get(`${API_URL}/users`, () =>
        HttpResponse.json(
          ok(
            deletedId ? defaultUsers.filter((u) => u.id !== deletedId) : defaultUsers,
            listMeta(deletedId ? defaultUsers.length - 1 : defaultUsers.length),
          ),
        ),
      ),
    );
    signInAs(testAdmin);
    renderUsers();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: `Actions for ${aliceUser.email}` }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));
    await user.click(await screen.findByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(deletedId).toBe(aliceUser.id);
    });
    await waitFor(() => {
      expect(screen.queryByText("Alice")).not.toBeInTheDocument();
    });
  });
});
