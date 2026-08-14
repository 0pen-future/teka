import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import {
  resetTeachingApiStore,
  seedPlan,
  teachingHandlers,
} from "@/features/teaching/__tests__/teaching-handlers";
import { API_URL, fail, ok } from "@/test/msw/handlers";
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
  localStorage.clear();
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

    // Entry links keep their document order inside the grouped sidebar nav.
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    const labels = within(sidebarNav)
      .getAllByRole("link")
      .map((link) => link.textContent);
    const students = labels.indexOf("Lớp & học sinh");
    const contacts = labels.indexOf("Phụ huynh");
    expect(students).toBeGreaterThanOrEqual(0);
    expect(contacts).toBe(students + 1);
  });
});

describe("grouped sidebar", () => {
  it("renders each prototype section as a group owning its entries", async () => {
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    // Duyệt giáo án is owner-gated behind /centers/me — the slowest entry to appear.
    await within(sidebarNav).findByRole("link", { name: "Duyệt giáo án" });

    const expected: Record<string, string[]> = {
      "Dạy học": ["Điểm danh", "Quản lý lớp học", "Hồ sơ học sinh", "Lớp & học sinh", "Phụ huynh"],
      "Học phí": ["Chốt sổ", "Gửi thông báo", "Thu tiền"],
      "Trung tâm": ["Duyệt giáo án", "Cài đặt trung tâm"],
    };
    for (const [header, labels] of Object.entries(expected)) {
      const group = within(sidebarNav).getByRole("group", { name: header });
      // Every entry (live link or disabled span) sits inside its own group.
      for (const label of labels) {
        expect(within(group).getByText(label)).toBeInTheDocument();
      }
    }
    // The center entry is the renamed settings link.
    expect(within(sidebarNav).getByRole("link", { name: "Cài đặt trung tâm" })).toHaveAttribute(
      "href",
      "/center",
    );
    expect(within(sidebarNav).queryByRole("link", { name: "Trung tâm" })).not.toBeInTheDocument();
  });

  it("shows the center card with name, prefix-stripped initial, and owner role", async () => {
    renderLayout();
    // Default /centers/me handler is owner-shaped for Trung Tâm Bình Minh.
    expect(await screen.findByText("Trung Tâm Bình Minh")).toBeInTheDocument();
    expect(screen.getByText("Chủ trung tâm")).toBeInTheDocument();
    // The disc drops the generic "Trung Tâm" prefix: Bình Minh → B.
    expect(screen.getByText("B")).toBeInTheDocument();
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
    expect(
      within(sheet)
        .getByRole("link", { name: "Gửi thông báo" })
        .getAttribute("href"),
    ).toMatch(/^\/notifications\//);
    const contacts = within(sheet).getByRole("link", { name: "Phụ huynh" });
    expect(contacts).toHaveAttribute("href", "/contacts");
    expect(within(sheet).getByRole("link", { name: "Cài đặt trung tâm" })).toHaveAttribute(
      "href",
      "/center",
    );

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

  it("lists the teaching v2 entries inside the Thêm sheet", async () => {
    const user = userEvent.setup();
    renderLayout();
    const { moreTab } = await findBottomNav();

    await user.click(moreTab);
    const sheet = await screen.findByRole("dialog");

    expect(within(sheet).getByRole("link", { name: "Quản lý lớp học" })).toHaveAttribute(
      "href",
      "/classbook",
    );
    expect(within(sheet).getByRole("link", { name: "Hồ sơ học sinh" })).toHaveAttribute(
      "href",
      "/records",
    );
    expect(await within(sheet).findByRole("link", { name: "Duyệt giáo án" })).toHaveAttribute(
      "href",
      "/lesson-plans",
    );
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

describe("teaching v2 nav", () => {
  it("orders Dạy học per the prototype and links the new entries", async () => {
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    await within(sidebarNav).findByRole("link", { name: "Quản lý lớp học" });

    const group = within(sidebarNav).getByRole("group", { name: "Dạy học" });
    const labels = within(group)
      .getAllByRole("link")
      .map((link) => link.textContent);
    expect(labels).toEqual([
      "Điểm danh",
      "Quản lý lớp học",
      "Hồ sơ học sinh",
      "Lớp & học sinh",
      "Phụ huynh",
    ]);
    expect(within(group).getByRole("link", { name: "Quản lý lớp học" })).toHaveAttribute(
      "href",
      "/classbook",
    );
    expect(within(group).getByRole("link", { name: "Hồ sơ học sinh" })).toHaveAttribute(
      "href",
      "/records",
    );
  });

  it("shows Duyệt giáo án to owners before Cài đặt trung tâm", async () => {
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    const link = await within(sidebarNav).findByRole("link", { name: "Duyệt giáo án" });
    expect(link).toHaveAttribute("href", "/lesson-plans");

    const group = within(sidebarNav).getByRole("group", { name: "Trung tâm" });
    const labels = within(group)
      .getAllByRole("link")
      .map((l) => l.textContent);
    expect(labels).toEqual(["Duyệt giáo án", "Cài đặt trung tâm"]);
  });

  it("hides Duyệt giáo án from non-owner members and never fetches their queue", async () => {
    let queueRequests = 0;
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
      ),
      http.get(`${API_URL}/teaching/review-queue`, () => {
        queueRequests += 1;
        return HttpResponse.json(fail("FORBIDDEN", "owner only"), { status: 403 });
      }),
    );
    renderLayout();
    // Member role label proves /centers/me resolved member-shaped.
    await screen.findByText("Giáo viên");
    expect(screen.queryByRole("link", { name: "Duyệt giáo án" })).not.toBeInTheDocument();
    // The nav-dot query is role-gated: a member must never hit the endpoint.
    expect(queueRequests).toBe(0);
  });

  it("marks Duyệt giáo án pending while a plan awaits review", async () => {
    resetTeachingApiStore();
    server.use(...teachingHandlers);
    seedPlan("class-1", 0, { status: "pending" });
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    const link = await within(sidebarNav).findByRole("link", { name: "Duyệt giáo án" });
    // The dot appears once the review-queue query resolves, after the link.
    await waitFor(() => expect(link.querySelector(".bg-coral-400")).not.toBeNull());
  });

  it("shows no pending dot on Duyệt giáo án when nothing awaits review", async () => {
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    const link = await within(sidebarNav).findByRole("link", { name: "Duyệt giáo án" });
    expect(link.querySelector(".bg-coral-400")).toBeNull();
  });
});
