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

/** A member-shaped `/centers/me` holding exactly `permissions`. */
function asMember(permissions: string[]) {
  server.use(
    http.get(`${API_URL}/centers/me`, () =>
      HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
    ),
  );
}

describe("Học sinh nav entry", () => {
  it("links to /students from the sidebar and the icon rail", async () => {
    renderLayout();

    // Sidebar (text label) + rail (aria-label); the bottom bar holds it only
    // inside the closed "Thêm" sheet, so exactly two links exist up front.
    const links = await screen.findAllByRole("link", { name: "Học sinh" });
    expect(links).toHaveLength(2);
    for (const link of links) {
      expect(link).toHaveAttribute("href", "/students");
    }
  });

  it("leads the Lớp học group", async () => {
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    const group = await within(sidebarNav).findByRole("group", { name: "Lớp học" });

    await within(group).findByRole("link", { name: "Học sinh" });
    expect(within(group).getAllByRole("link")[0]).toHaveTextContent("Học sinh");
  });

  it("replaces the retired contacts, import and student-admin entries everywhere", async () => {
    const user = userEvent.setup();
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    await within(sidebarNav).findByRole("link", { name: "Duyệt giáo án" });

    const { moreTab } = await findBottomNav();
    await user.click(moreTab);
    const sheet = await screen.findByRole("dialog");
    await within(sheet).findByRole("link", { name: "Học sinh" });

    for (const label of ["Phụ huynh", "Nhập từ Excel", "Quản trị học sinh"]) {
      expect(within(sidebarNav).queryByText(label)).not.toBeInTheDocument();
      expect(within(sheet).queryByText(label)).not.toBeInTheDocument();
    }
    expect(within(sidebarNav).queryByText("Giảng dạy")).not.toBeInTheDocument();
  });

  it("shows the entry to a member holding students.list", async () => {
    asMember(["students.list"]);
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;

    expect(await within(sidebarNav).findByRole("link", { name: "Học sinh" })).toHaveAttribute(
      "href",
      "/students",
    );
  });

  it("hides the entry from a member without students.list", async () => {
    asMember(["sessions.list"]);
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;

    await within(sidebarNav).findByRole("link", { name: "Điểm danh" });
    expect(within(sidebarNav).queryByRole("link", { name: "Học sinh" })).not.toBeInTheDocument();
  });

  it("drops the header of a group whose every entry is filtered away", async () => {
    // Every Học liệu and Học phí entry is gated on a key this member lacks.
    asMember(["sessions.list"]);
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;

    await within(sidebarNav).findByRole("link", { name: "Điểm danh" });
    expect(within(sidebarNav).getByRole("group", { name: "Dạy học" })).toBeInTheDocument();
    // Lời mời nhận lớp is ungated, so Lớp học stays.
    expect(within(sidebarNav).getByRole("group", { name: "Lớp học" })).toBeInTheDocument();
    expect(within(sidebarNav).queryByRole("group", { name: "Học liệu" })).not.toBeInTheDocument();
    expect(within(sidebarNav).queryByRole("group", { name: "Học phí" })).not.toBeInTheDocument();
  });
});

describe("grouped sidebar", () => {
  it("renders each prototype section as a group owning its entries", async () => {
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    // Duyệt giáo án is owner-gated behind /centers/me — the slowest entry to appear.
    await within(sidebarNav).findByRole("link", { name: "Duyệt giáo án" });

    const expected: Record<string, string[]> = {
      "Dạy học": ["Điểm danh", "Sổ lớp"],
      "Học phí": ["Chốt sổ", "Gửi thông báo", "Thu tiền"],
      "Trung tâm": [
        "Duyệt giáo án",
        "Nhật ký hoạt động",
        "Công việc",
        "Phân quyền vai trò",
        "Cài đặt trung tâm",
      ],
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
  it("keeps three primary tabs plus a Thêm tab", async () => {
    renderLayout();
    const { nav } = await findBottomNav();

    // Thu tiền needs the current period; wait until its link resolves.
    await within(nav).findByRole("link", { name: "Thu tiền" });

    const tabLabels = within(nav)
      .getAllByRole("link")
      .map((link) => link.textContent);
    expect(tabLabels).toEqual(["Tổng quan", "Điểm danh", "Thu tiền"]);
    expect(within(nav).queryByText("Chốt sổ")).not.toBeInTheDocument();
    expect(within(nav).queryByText("Gửi thông báo")).not.toBeInTheDocument();
    // Reachable through the Thêm sheet, never a primary tab.
    expect(within(nav).queryByText("Học sinh")).not.toBeInTheDocument();
  });

  it("opens the Thêm sheet listing the overflow entries and navigates from it", async () => {
    const user = userEvent.setup();
    const { router } = renderLayout();
    const { moreTab } = await findBottomNav();

    await user.click(moreTab);
    const sheet = await screen.findByRole("dialog");

    const billing = await within(sheet).findByRole("link", { name: "Chốt sổ" });
    expect(billing.getAttribute("href")).toMatch(/^\/billing\//);
    expect(within(sheet).getByRole("link", { name: "Gửi thông báo" })).toHaveAttribute(
      "href",
      "/reports",
    );
    const students = await within(sheet).findByRole("link", { name: "Học sinh" });
    expect(students).toHaveAttribute("href", "/students");
    expect(within(sheet).getByRole("link", { name: "Cài đặt trung tâm" })).toHaveAttribute(
      "href",
      "/center",
    );

    await user.click(students);
    await waitFor(() => expect(router.state.location.pathname).toBe("/students"));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("keeps Học sinh active on /students/import, which has no entry of its own", async () => {
    renderLayout("/students/import");
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;

    const studentsLink = await within(sidebarNav).findByRole("link", { name: "Học sinh" });
    expect(studentsLink).toHaveAttribute("aria-current", "page");
  });

  it("keeps only Phân quyền vai trò active on /center/permissions, not Cài đặt trung tâm", async () => {
    renderLayout("/center/permissions");
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;

    const permissionsLink = await within(sidebarNav).findByRole("link", {
      name: "Phân quyền vai trò",
    });
    const centerLink = within(sidebarNav).getByRole("link", { name: "Cài đặt trung tâm" });
    // /center/permissions is a subpath of /center; only the deeper entry lights up.
    expect(permissionsLink).toHaveAttribute("aria-current", "page");
    expect(centerLink).not.toHaveAttribute("aria-current");
  });

  it("keeps the parent entry active on a student detail route", async () => {
    renderLayout("/students/42");
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;

    const studentsLink = await within(sidebarNav).findByRole("link", { name: "Học sinh" });
    // A genuine child (/students/:id) still highlights its parent nav entry.
    expect(studentsLink).toHaveAttribute("aria-current", "page");
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

    expect(within(sheet).getByRole("link", { name: "Sổ lớp" })).toHaveAttribute(
      "href",
      "/classbook",
    );
    expect(await within(sheet).findByRole("link", { name: "Duyệt giáo án" })).toHaveAttribute(
      "href",
      "/lesson-plans",
    );
    // The retired duplicates are gone from the sheet too.
    for (const label of ["Hồ sơ học sinh", "Lớp cần tuyển sinh", "Cấu hình lớp học"]) {
      expect(within(sheet).queryByText(label)).not.toBeInTheDocument();
    }
    // The owner-only roster entry lives here too, never on the primary tabs.
    expect(await within(sheet).findByRole("link", { name: "Học sinh" })).toHaveAttribute(
      "href",
      "/students",
    );
  });

  it("marks Thêm active on /students now that the entry moved to the sheet", async () => {
    renderLayout("/students");
    const { moreTab } = await findBottomNav();
    expect(moreTab).toHaveClass("text-mint-600");
  });

  it("renders only the period-scoped entries disabled while no period resolves", async () => {
    server.use(
      http.post(`${API_URL}/billing-periods`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "boom"), { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    renderLayout();
    const { moreTab, nav } = await findBottomNav();
    expect(await within(nav).findByText("Thu tiền")).toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Thu tiền" })).not.toBeInTheDocument();

    await user.click(moreTab);
    const sheet = await screen.findByRole("dialog");

    expect(within(sheet).getByText("Chốt sổ")).toBeInTheDocument();
    expect(within(sheet).queryByRole("link", { name: "Chốt sổ" })).not.toBeInTheDocument();
    // Gửi thông báo opens the period list, so it never waits on the period.
    expect(within(sheet).getByRole("link", { name: "Gửi thông báo" })).toHaveAttribute(
      "href",
      "/reports",
    );
    // Học sinh is not period-scoped and stays a live link.
    expect(await within(sheet).findByRole("link", { name: "Học sinh" })).toBeInTheDocument();
  });
});

describe("teaching v2 nav", () => {
  it("orders Dạy học per the prototype and links the new entries", async () => {
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    await within(sidebarNav).findByRole("link", { name: "Sổ lớp" });

    const group = within(sidebarNav).getByRole("group", { name: "Dạy học" });
    const labels = within(group)
      .getAllByRole("link")
      .map((link) => link.textContent);
    expect(labels).toEqual(["Điểm danh", "Sổ lớp"]);
    expect(within(group).getByRole("link", { name: "Sổ lớp" })).toHaveAttribute(
      "href",
      "/classbook",
    );
    // Student records live on the students page; the duplicate entry is gone.
    expect(within(sidebarNav).queryByText("Hồ sơ học sinh")).not.toBeInTheDocument();
  });

  it("shows Sổ lớp to a member holding teaching.read", async () => {
    asMember(["teaching.read"]);
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    expect(await within(sidebarNav).findByRole("link", { name: "Sổ lớp" })).toHaveAttribute(
      "href",
      "/classbook",
    );
  });

  it("hides Sổ lớp from a member without teaching.read, even with classes.list", async () => {
    asMember(["classes.list", "sessions.list"]);
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    await within(sidebarNav).findByRole("link", { name: "Điểm danh" });
    expect(within(sidebarNav).queryByText("Sổ lớp")).not.toBeInTheDocument();
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
    expect(labels).toEqual([
      "Duyệt giáo án",
      "Nhật ký hoạt động",
      "Công việc",
      "Phân quyền vai trò",
      "Cài đặt trung tâm",
    ]);
    // Score sets are applied from the program template; the owner-only entry is gone.
    expect(screen.queryByText("Cấu hình lớp học")).not.toBeInTheDocument();
  });

  it("shows Phân quyền vai trò to owners linking /center/permissions", async () => {
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    expect(
      await within(sidebarNav).findByRole("link", { name: "Phân quyền vai trò" }),
    ).toHaveAttribute("href", "/center/permissions");
  });

  it("hides Phân quyền vai trò from non-owner members", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
      ),
    );
    renderLayout();
    await screen.findByText("Giáo viên");
    expect(screen.queryByRole("link", { name: "Phân quyền vai trò" })).not.toBeInTheDocument();
  });

  it("shows Nhật ký hoạt động to owners linking /audit", async () => {
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    expect(
      await within(sidebarNav).findByRole("link", { name: "Nhật ký hoạt động" }),
    ).toHaveAttribute("href", "/audit");
  });

  it("hides Nhật ký hoạt động from non-owner members", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
      ),
    );
    renderLayout();
    // Member role label proves /centers/me resolved member-shaped.
    await screen.findByText("Giáo viên");
    expect(screen.queryByRole("link", { name: "Nhật ký hoạt động" })).not.toBeInTheDocument();
  });

  it("shows Công việc to owners linking /tasks, including inside the Thêm sheet", async () => {
    const user = userEvent.setup();
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    expect(await within(sidebarNav).findByRole("link", { name: "Công việc" })).toHaveAttribute(
      "href",
      "/tasks",
    );

    const { moreTab } = await findBottomNav();
    await user.click(moreTab);
    const sheet = await screen.findByRole("dialog");
    expect(within(sheet).getByRole("link", { name: "Công việc" })).toHaveAttribute(
      "href",
      "/tasks",
    );
  });

  it("hides Công việc from a member without tasks.list", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
      ),
    );
    renderLayout();
    // Member role label proves /centers/me resolved member-shaped.
    await screen.findByText("Giáo viên");
    expect(screen.queryByRole("link", { name: "Công việc" })).not.toBeInTheDocument();
  });

  it("shows Công việc to a member holding tasks.list", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions: ["tasks.list"] })),
      ),
    );
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    expect(await within(sidebarNav).findByRole("link", { name: "Công việc" })).toHaveAttribute(
      "href",
      "/tasks",
    );
  });

  it("shows granted surfaces to a member holding the matching permissions", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(
          ok({
            center_name: "Trung Tâm Bình Minh",
            permissions: ["audit.read", "students.list"],
          }),
        ),
      ),
    );
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    expect(
      await within(sidebarNav).findByRole("link", { name: "Nhật ký hoạt động" }),
    ).toHaveAttribute("href", "/audit");
    expect(within(sidebarNav).getByRole("link", { name: "Học sinh" })).toHaveAttribute(
      "href",
      "/students",
    );
    // Keys the member does not hold stay hidden.
    expect(screen.queryByRole("link", { name: "Duyệt giáo án" })).not.toBeInTheDocument();
  });

  it("shows main entries only for the member's effective keys", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(
          ok({
            center_name: "Trung Tâm Bình Minh",
            permissions: ["sessions.list", "students.list"],
          }),
        ),
      ),
    );
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;

    expect(await within(sidebarNav).findByRole("link", { name: "Điểm danh" })).toHaveAttribute(
      "href",
      "/sessions",
    );
    // students.list alone opens the unified students page.
    expect(within(sidebarNav).getByRole("link", { name: "Học sinh" })).toBeInTheDocument();
    // Entries whose route key the member lacks disappear entirely — no
    // disabled placeholder that would only 403 on click.
    expect(within(sidebarNav).queryByText("Sổ lớp")).not.toBeInTheDocument();
    expect(within(sidebarNav).queryByText("Chốt sổ")).not.toBeInTheDocument();
    expect(within(sidebarNav).queryByText("Thu tiền")).not.toBeInTheDocument();
    expect(within(sidebarNav).queryByText("Gửi thông báo")).not.toBeInTheDocument();
    // Ungated entries stay for every member.
    expect(within(sidebarNav).getByRole("link", { name: "Tổng quan" })).toBeInTheDocument();
    expect(within(sidebarNav).getByRole("link", { name: "Cài đặt trung tâm" })).toBeInTheDocument();
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

  it("gives the owner exactly one Gửi thông báo entry, linking the period list", async () => {
    const user = userEvent.setup();
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    // Owner-shaped default: wait for the slowest owner entry to appear first.
    await within(sidebarNav).findByRole("link", { name: "Duyệt giáo án" });

    const links = within(sidebarNav).getAllByRole("link", { name: "Gửi thông báo" });
    expect(links).toHaveLength(1);
    expect(links[0]).toHaveAttribute("href", "/reports");
    const group = within(sidebarNav).getByRole("group", { name: "Học phí" });
    expect(within(group).getByRole("link", { name: "Gửi thông báo" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Gửi báo cáo" })).not.toBeInTheDocument();

    // The entry also reaches the mobile Thêm sheet, never the primary tabs.
    const { moreTab } = await findBottomNav();
    await user.click(moreTab);
    const sheet = await screen.findByRole("dialog");
    expect(within(sheet).getAllByRole("link", { name: "Gửi thông báo" })).toHaveLength(1);
    expect(within(sheet).queryByRole("link", { name: "Gửi báo cáo" })).not.toBeInTheDocument();
  });

  it("gives a member holding reports.send the same single entry", async () => {
    asMember(["reports.send"]);
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;

    await within(sidebarNav).findByRole("link", { name: "Gửi thông báo" });
    const links = within(sidebarNav).getAllByRole("link", { name: "Gửi thông báo" });
    expect(links).toHaveLength(1);
    expect(links[0]).toHaveAttribute("href", "/reports");
    expect(screen.queryByRole("link", { name: "Gửi báo cáo" })).not.toBeInTheDocument();
  });

  it("hides Gửi thông báo from a plain member without reports.send", async () => {
    asMember([]);
    renderLayout();
    // Member role label proves /centers/me resolved member-shaped.
    await screen.findByText("Giáo viên");
    expect(screen.queryByText("Gửi thông báo")).not.toBeInTheDocument();
    expect(screen.queryByText("Gửi báo cáo")).not.toBeInTheDocument();
  });

  it("keeps Gửi thông báo active on the period list and on a period's send page", async () => {
    for (const route of ["/reports", "/notifications/p1"]) {
      const { unmount } = renderLayout(route);
      const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
      const send = await within(sidebarNav).findByRole("link", { name: "Gửi thông báo" });
      expect(send).toHaveAttribute("aria-current", "page");
      unmount();
    }
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

describe("Lớp học nav group", () => {
  it("shows Danh mục lớp to owners in its own group after Dạy học", async () => {
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    const group = await within(sidebarNav).findByRole("group", { name: "Lớp học" });

    // Permission-gated entries appear once /centers/me resolves.
    expect(await within(group).findByRole("link", { name: "Danh mục lớp" })).toHaveAttribute(
      "href",
      "/classes",
    );
    const teaching = within(sidebarNav).getByRole("group", { name: "Dạy học" });
    expect(teaching.compareDocumentPosition(group) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("has no Lớp cần tuyển sinh entry; the catalog owns the recruiting view", async () => {
    renderLayout("/classes");
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    const group = await within(sidebarNav).findByRole("group", { name: "Lớp học" });
    await within(group).findByRole("link", { name: "Danh mục lớp" });
    expect(within(sidebarNav).queryByText("Lớp cần tuyển sinh")).not.toBeInTheDocument();
  });

  it("shows the entry to a member holding classes.list", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(
          ok({ center_name: "Trung Tâm Bình Minh", permissions: ["classes.list"] }),
        ),
      ),
    );
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    expect(await within(sidebarNav).findByRole("link", { name: "Danh mục lớp" })).toHaveAttribute(
      "href",
      "/classes",
    );
  });

  it("hides the entry from a member without classes.list", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(
          ok({ center_name: "Trung Tâm Bình Minh", permissions: ["sessions.list"] }),
        ),
      ),
    );
    renderLayout();
    await screen.findByText("Giáo viên");
    expect(screen.queryByText("Danh mục lớp")).not.toBeInTheDocument();
  });

  it("lists the entry inside the Thêm sheet and marks Thêm active on a class route", async () => {
    const user = userEvent.setup();
    renderLayout("/classes/abc");
    const { moreTab } = await findBottomNav();
    expect(moreTab).toHaveClass("text-mint-600");

    await user.click(moreTab);
    const sheet = await screen.findByRole("dialog");
    expect(await within(sheet).findByRole("link", { name: "Danh mục lớp" })).toHaveAttribute(
      "href",
      "/classes",
    );
  });
});

describe("Học liệu nav group", () => {
  it("shows its own group after Lớp học with the prototype's entries in order", async () => {
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    const teaching = await within(sidebarNav).findByRole("group", { name: "Lớp học" });
    // Its entries are all gated, so the header waits for /centers/me.
    const group = await within(sidebarNav).findByRole("group", { name: "Học liệu" });
    expect(teaching.compareDocumentPosition(group) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    await within(group).findByRole("link", { name: "Kho học liệu" });
    const links = within(group).getAllByRole("link");
    expect(links.map((link) => [link.textContent, link.getAttribute("href")])).toEqual([
      ["Lộ trình học", "/paths"],
      ["Khóa học", "/courses"],
      ["Kho học liệu", "/library"],
    ]);
    // Lớp học keeps the students page and the class entries.
    expect(
      within(teaching)
        .getAllByRole("link")
        .map((link) => link.textContent),
    ).toEqual(["Học sinh", "Danh mục lớp", "Lời mời nhận lớp"]);
    expect(screen.queryByText("Danh mục khóa học")).not.toBeInTheDocument();
    expect(screen.queryByText("Ngân hàng nội dung")).not.toBeInTheDocument();
  });

  it("gates each entry on its own read key", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(
          ok({ center_name: "Trung Tâm Bình Minh", permissions: ["classes.list", "courses.read"] }),
        ),
      ),
    );
    renderLayout();
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    const group = await within(sidebarNav).findByRole("group", { name: "Học liệu" });
    await within(group).findByRole("link", { name: "Khóa học" });
    expect(within(group).getAllByRole("link")).toHaveLength(1);
    expect(screen.queryByText("Lộ trình học")).not.toBeInTheDocument();
  });

  it("lists the entries inside the Thêm sheet", async () => {
    const user = userEvent.setup();
    renderLayout();
    const { moreTab } = await findBottomNav();

    await user.click(moreTab);
    const sheet = await screen.findByRole("dialog");
    expect(await within(sheet).findByRole("link", { name: "Kho học liệu" })).toHaveAttribute(
      "href",
      "/library",
    );
    for (const [name, href] of [
      ["Lộ trình học", "/paths"],
      ["Khóa học", "/courses"],
    ]) {
      expect(within(sheet).getByRole("link", { name })).toHaveAttribute("href", href);
    }
  });

  it("keeps Kho học liệu active on the content bank and template routes", async () => {
    for (const route of ["/library/materials", "/library/templates/1"]) {
      const { unmount } = renderLayout(route);
      const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
      const hub = await within(sidebarNav).findByRole("link", { name: "Kho học liệu" });
      expect(hub).toHaveAttribute("aria-current", "page");
      unmount();
    }
  });

  it("marks Khóa học active on a course detail route", async () => {
    renderLayout("/courses/c1");
    const sidebarNav = screen.getAllByRole("navigation", { name: "Main" })[0]!;
    const courses = await within(sidebarNav).findByRole("link", { name: "Khóa học" });
    expect(courses).toHaveAttribute("aria-current", "page");
  });
});

describe("current period disc", () => {
  it("shows the rail's period disc to a member holding billing.read", async () => {
    asMember(["billing.read"]);
    renderLayout();
    expect(await screen.findByRole("link", { name: /^Kỳ hiện tại: tháng / })).toHaveAttribute(
      "href",
      expect.stringMatching(/^\/billing\//),
    );
  });

  it("hides the rail's period disc from a member without billing.read", async () => {
    asMember(["sessions.list"]);
    renderLayout();
    // The sidebar period card proves the current period resolved.
    await screen.findByText(/^Tháng \d+\/\d+$/);
    expect(screen.queryByRole("link", { name: /^Kỳ hiện tại: tháng / })).not.toBeInTheDocument();
  });
});
