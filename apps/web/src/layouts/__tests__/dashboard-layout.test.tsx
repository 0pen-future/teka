import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { DashboardLayout } from "../dashboard-layout";

function renderLayout(route = "/") {
  signInAs(testPrimaryTeacher);
  // "*" mounts the layout at every location so NavLink active state follows `route`.
  return renderWithProviders(<DashboardLayout />, { route, path: "*" });
}

/** The bottom tab bar is the nav that owns the "Thêm" tab. */
async function findBottomNav() {
  const moreTab = await screen.findByRole("button", { name: "Thêm" });
  const nav = moreTab.closest("nav");
  expect(nav).not.toBeNull();
  return { moreTab, nav: nav! };
}

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("Phụ huynh nav entry", () => {
  it("links to /contacts from the sidebar and the icon rail", async () => {
    renderLayout();

    // Sidebar (text label) + rail (aria-label); the bottom bar holds it only
    // inside the closed "Thêm" sheet, so exactly two links exist up front.
    const links = await screen.findAllByRole("link", { name: "Phụ huynh" });
    expect(links).toHaveLength(2);
    for (const link of links) {
      expect(link).toHaveAttribute("href", "/contacts");
    }
  });

  it("sits after Lớp & học sinh in the sidebar order", async () => {
    renderLayout();
    await screen.findAllByRole("link", { name: "Phụ huynh" });

    // Each sidebar nav child is one entry (link or disabled span) whose only
    // text is its label — icons are svg-only.
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    const labels = Array.from(sidebarNav.children).map((el) => el.textContent);
    const students = labels.indexOf("Lớp & học sinh");
    const contacts = labels.indexOf("Phụ huynh");
    expect(students).toBeGreaterThanOrEqual(0);
    expect(contacts).toBe(students + 1);
  });
});

describe("bottom tab bar", () => {
  it("keeps four primary tabs plus a Thêm tab", async () => {
    renderLayout();
    const { nav } = await findBottomNav();

    // Thu tiền needs the current period; wait until its link resolves.
    await within(nav).findByRole("link", { name: "Thu tiền" });

    const tabLabels = within(nav)
      .getAllByRole("link")
      .map((link) => link.textContent);
    expect(tabLabels).toEqual(["Tổng quan", "Điểm danh", "Lớp & học sinh", "Thu tiền"]);
    expect(within(nav).queryByText("Chốt sổ")).not.toBeInTheDocument();
    expect(within(nav).queryByText("Gửi thông báo")).not.toBeInTheDocument();
    expect(within(nav).queryByText("Phụ huynh")).not.toBeInTheDocument();
  });

  it("opens the Thêm sheet listing the overflow entries and navigates from it", async () => {
    const user = userEvent.setup();
    const { router } = renderLayout();
    const { moreTab } = await findBottomNav();

    await user.click(moreTab);
    const sheet = await screen.findByRole("dialog");

    const billing = await within(sheet).findByRole("link", { name: "Chốt sổ" });
    expect(billing.getAttribute("href")).toMatch(/^\/billing\//);
    expect(within(sheet).getByRole("link", { name: "Gửi thông báo" }).getAttribute("href")).toMatch(
      /^\/notifications\//,
    );
    const contacts = within(sheet).getByRole("link", { name: "Phụ huynh" });
    expect(contacts).toHaveAttribute("href", "/contacts");

    await user.click(contacts);
    await waitFor(() => expect(router.state.location.pathname).toBe("/contacts"));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("marks Thêm active while on an overflow route", async () => {
    renderLayout("/contacts");
    const { moreTab } = await findBottomNav();
    expect(moreTab).toHaveClass("text-mint-600");
  });

  it("does not mark Thêm active on a primary route", async () => {
    renderLayout("/");
    const { moreTab } = await findBottomNav();
    expect(moreTab).not.toHaveClass("text-mint-600");
  });

  it("renders period-scoped sheet entries disabled while no period resolves", async () => {
    server.use(
      http.post(`${API_URL}/billing-periods`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "boom"), { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    renderLayout();
    const { moreTab } = await findBottomNav();

    await user.click(moreTab);
    const sheet = await screen.findByRole("dialog");

    expect(within(sheet).getByText("Chốt sổ")).toBeInTheDocument();
    expect(within(sheet).queryByRole("link", { name: "Chốt sổ" })).not.toBeInTheDocument();
    expect(within(sheet).queryByRole("link", { name: "Gửi thông báo" })).not.toBeInTheDocument();
    // Phụ huynh is not period-scoped and stays a live link.
    expect(within(sheet).getByRole("link", { name: "Phụ huynh" })).toBeInTheDocument();
  });
});
