import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";
import { mockViewport } from "@/test/viewport";

import { StudentsPage } from "../pages/students-page";
import {
  classWithSchedule,
  dayOfCurrentMonth,
  enrollmentActive,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
  studentOnlyChild,
  studentSiblingTwo,
} from "./roster-handlers";

/** Six active classes — one past the threshold that reveals "Tìm lớp…". */
function sixClasses() {
  return [
    classWithSchedule,
    ...["Toán 7B", "Văn 6A", "Văn 8C", "Anh 9A", "Lý 8B"].map((name, index) => ({
      ...classWithSchedule,
      id: `70000000-0000-4000-8000-00000000001${index}`,
      name,
    })),
  ];
}

function useSixClasses() {
  server.use(
    http.get(`${API_URL}/classes`, () => {
      const items = sixClasses();
      return HttpResponse.json(ok(items, listMeta(items.length)));
    }),
  );
}

function renderStudentsPage(route = "/students") {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<StudentsPage />, { route, path: "/students" });
}

function pageTabs() {
  return screen.getByRole("tablist", { name: "Khu vực" });
}

function classPills() {
  return screen.getByRole("tablist", { name: "Lớp" });
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
  // Desktop: the enroll dialog's class picker opens as a popover, not the sheet.
  mockViewport(1024);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("StudentsPage tabs", () => {
  it("opens a bare URL on every student of the center, without the enrollment columns", async () => {
    renderStudentsPage();

    const tabs = await screen.findByRole("tablist", { name: "Khu vực" });
    expect(await within(tabs).findByRole("tab", { name: "Tất cả (3)" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    const table = await screen.findByRole("table");
    expect(await within(table).findByRole("link", { name: "Trần Minh Khôi" })).toBeInTheDocument();
    expect(within(table).getAllByRole("link", { name: "Nguyễn Văn An" })).toHaveLength(2);
    expect(screen.getAllByRole("columnheader").map((th) => th.textContent)).toEqual([
      "Học sinh",
      "Người liên hệ",
      "Ghi chú và hành động",
    ]);
    // The class pills belong to the by-class tab only.
    expect(screen.queryByRole("tablist", { name: "Lớp" })).not.toBeInTheDocument();
  });

  it("issues no class, session or enrollment queries on the all tab", async () => {
    const requested: string[] = [];
    const onRequest = ({ request }: { request: Request }) => {
      requested.push(request.url);
    };
    server.events.on("request:start", onRequest);
    try {
      renderStudentsPage();
      await within(await screen.findByRole("table")).findByRole("link", {
        name: "Trần Minh Khôi",
      });
      expect(
        requested.filter(
          (url) =>
            url.includes("/classes") || url.includes("/sessions") || url.includes("/enrollments"),
        ),
      ).toEqual([]);
    } finally {
      server.events.removeListener("request:start", onRequest);
    }
  });

  it("switches panels when clicking the page tabs", async () => {
    renderStudentsPage();

    await screen.findByRole("tablist", { name: "Khu vực" });
    await userEvent.click(within(pageTabs()).getByRole("tab", { name: "Theo lớp" }));
    // The by-class panel brings the class pill strip with it.
    expect(await screen.findByRole("tablist", { name: "Lớp" })).toBeInTheDocument();
    expect(within(pageTabs()).getByRole("tab", { name: "Theo lớp" })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    await userEvent.click(within(pageTabs()).getByRole("tab", { name: "Người liên hệ" }));
    const contacts = await screen.findByRole("list", { name: "Danh sách người liên hệ" });
    expect(await within(contacts).findByText("Phạm Văn Hùng")).toBeInTheDocument();
    expect(screen.queryByRole("tablist", { name: "Lớp" })).not.toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("opens the by-class tab for a legacy ?class_id= link without a tab param", async () => {
    renderStudentsPage(`/students?class_id=${classWithSchedule.id}`);

    const pill = await screen.findByRole("tab", { name: "Toán 6A" });
    expect(pill).toHaveAttribute("aria-selected", "true");
    expect(within(pageTabs()).getByRole("tab", { name: "Theo lớp" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it.each(["classes", "students"])("maps the retired ?tab=%s to the by-class tab", async (tab) => {
    renderStudentsPage(`/students?tab=${tab}`);

    expect(await screen.findByRole("tab", { name: "Toán 6A" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(within(pageTabs()).getByRole("tab", { name: "Theo lớp" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("falls back to the all tab on an unknown ?tab= value", async () => {
    renderStudentsPage("/students?tab=bogus");

    const tabs = await screen.findByRole("tablist", { name: "Khu vực" });
    expect(await within(tabs).findByRole("tab", { name: "Tất cả (3)" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("filters the list by the ?q= search from the URL", async () => {
    renderStudentsPage("/students?q=khôi");

    expect(await screen.findByRole("searchbox", { name: "Tìm theo tên học sinh" })).toHaveValue(
      "khôi",
    );
    const table = await screen.findByRole("table");
    expect(await within(table).findByRole("link", { name: "Trần Minh Khôi" })).toBeInTheDocument();
    expect(within(table).queryByRole("link", { name: "Nguyễn Văn An" })).not.toBeInTheDocument();
  });
});

describe("StudentsPage v2 layout", () => {
  beforeEach(() => {
    // Freeze only Date (setTimeout stays real for msw/userEvent): the BUỔI
    // column and the session fixtures both derive from "today", so a real
    // clock would move the expected counts as the month progresses.
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-08-20T10:00:00"));
    // Re-seed under the frozen clock so fixture dates land in the same month.
    resetRosterStore();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("shows the v2 data-minimization subtitle", async () => {
    renderStudentsPage();

    await screen.findByRole("tablist", { name: "Khu vực" });
    expect(
      screen.getByText(
        "Chỉ lưu: họ tên · ngày nhập học · lớp · người liên hệ. Không thu thập gì thêm.",
      ),
    ).toBeInTheDocument();
  });

  it("renders the v2 columns with the current month in the sessions header", async () => {
    renderStudentsPage("/students?tab=by-class");

    await screen.findByRole("table");
    const headers = screen.getAllByRole("columnheader").map((th) => th.textContent);
    expect(headers).toEqual([
      "Học sinh",
      "Người liên hệ",
      "Nhập học",
      "Buổi T8",
      "Ghi chú và hành động",
    ]);
  });

  it("keeps only the roster actions in the students-tab header, in order", async () => {
    renderStudentsPage("/students?tab=by-class");

    // Owner-gated: waits for `/centers/me` to resolve, not just `/classes`.
    const enrollExisting = await screen.findByRole("button", { name: "+ Ghi danh học sinh" });
    const addStudent = await screen.findByRole("button", { name: "+ Thêm học sinh" });
    // Enroll sits right before add-student: picking an existing student is
    // offered before creating a new record, to steer away from duplicates.
    expect(enrollExisting.compareDocumentPosition(addStudent)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    // Class management moved into the classes tab: no settings link and no
    // create-class button up here anymore. The member-only report-send button
    // died with member access — the reports/notifications screens own sending.
    expect(screen.queryByRole("link", { name: "⚙ Cài đặt lớp" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "+ Tạo lớp mới" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Gửi báo cáo" })).not.toBeInTheDocument();
  });

  it("shows enrollment start and this month's non-cancelled session count", async () => {
    renderStudentsPage("/students?tab=by-class");

    const table = await screen.findByRole("table");
    const link = await within(table).findByRole("link", { name: "Nguyễn Văn An" });
    const row = link.closest("tr")!;
    // Enrolled since 2026-01-05, so every non-cancelled session up to the
    // frozen "today" (Aug 20) counts: days 5, 12, 19. The cancelled day-8
    // session is skipped and day 26 is beyond the queried range — the
    // sessions GET generates missing rows server-side, so the page must not
    // ask for future dates.
    expect(within(row).getByText("05/01")).toBeInTheDocument();
    expect(await within(row).findByText("3")).toBeInTheDocument();
  });

  it("counts only sessions inside the student's enrollment window", async () => {
    getRosterStore().enrollments.push({
      ...enrollmentActive,
      id: "80000000-0000-4000-8000-000000000002",
      student_id: studentSiblingTwo.id,
      student_name: studentSiblingTwo.full_name,
      started_on: dayOfCurrentMonth(15),
    });
    renderStudentsPage("/students?tab=by-class");

    const table = await screen.findByRole("table");
    // Two enrolled students share the same full name; the display-note badge
    // is the row's distinguishing mark.
    const badge = await within(table).findByText("Em, lớp 7B");
    const row = badge.closest("tr")!;
    // Joined day 15: of the queried sessions (up to Aug 20) only day 19
    // falls inside the enrollment window.
    expect(await within(row).findByText("1")).toBeInTheDocument();
    expect(within(row).getByText("15/08")).toBeInTheDocument();
  });

  it("issues no per-class queries on the unenrolled tab", async () => {
    const requested: string[] = [];
    const onRequest = ({ request }: { request: Request }) => {
      requested.push(request.url);
    };
    server.events.on("request:start", onRequest);
    try {
      // Legacy sentinel deep link — must still resolve to the unenrolled tab.
      renderStudentsPage("/students?class_id=none");
      await screen.findAllByRole("link", { name: "Trần Minh Khôi" });
      expect(within(pageTabs()).getByRole("tab", { name: /^Chưa vào lớp/ })).toHaveAttribute(
        "aria-selected",
        "true",
      );
      // No class is selected: neither the sessions nor the enrollments
      // query may fire (acceptance criterion 3 — no hidden fan-out).
      expect(
        requested.filter((url) => url.includes("/sessions") || url.includes("/enrollments")),
      ).toEqual([]);
    } finally {
      server.events.removeListener("request:start", onRequest);
    }
  });

  it("renders dashes for enrollment data on the unenrolled tab", async () => {
    renderStudentsPage();

    await screen.findByRole("tablist", { name: "Khu vực" });
    await userEvent.click(within(pageTabs()).getByRole("tab", { name: "Chưa vào lớp" }));
    const table = await screen.findByRole("table");
    const link = await within(table).findByRole("link", { name: "Trần Minh Khôi" });
    const row = link.closest("tr")!;
    // No enrollment for this tab: both NHẬP HỌC and BUỔI T{m} degrade to "—".
    expect(within(row).getAllByText("—")).toHaveLength(2);
  });
});

describe("StudentsPage roster flows", () => {
  it("opens the edit dialog from Sửa and the anonymize dialog from Xoá", async () => {
    renderStudentsPage("/students?tab=by-class");

    // Anchor on the class-scoped row: the first students fetch runs before
    // the classes list resolves the default class, so the initial unscoped
    // rows are replaced once the scope narrows — a click must wait for the
    // settled row, not the transient one.
    const table = await screen.findByRole("table");
    const row = (await within(table).findByRole("link", { name: "Nguyễn Văn An" })).closest("tr")!;
    await userEvent.click(within(row).getByRole("button", { name: "Sửa" }));
    expect(await screen.findByRole("dialog", { name: "Sửa học sinh" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Hủy" }));

    await userEvent.click(within(row).getByRole("button", { name: "Xoá" }));
    expect(await screen.findByRole("dialog", { name: "Xoá học sinh" })).toBeInTheDocument();
  });

  it("enrolling from the unenrolled tab lands on the class's students tab", async () => {
    renderStudentsPage("/students?tab=unenrolled");

    const table = await screen.findByRole("table");
    const row = (await within(table).findByRole("link", { name: "Trần Minh Khôi" })).closest("tr")!;
    await userEvent.click(within(row).getByRole("button", { name: "Ghi danh vào lớp" }));
    const dialog = await screen.findByRole("dialog", { name: "Ghi danh vào lớp" });
    await userEvent.click(within(dialog).getByRole("combobox", { name: "Lớp" }));
    await userEvent.click(await screen.findByRole("option", { name: /Toán 6A/ }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Ghi danh vào lớp" }));

    // onSuccess switches tab AND selects the enrollment's class, so the new
    // student is visible immediately in the class just enrolled into.
    expect(await screen.findByRole("tab", { name: "Toán 6A" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(within(pageTabs()).getByRole("tab", { name: "Theo lớp" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("postponing step 2 of the add-student wizard opens the unenrolled tab", async () => {
    renderStudentsPage("/students?tab=by-class");

    await userEvent.click(await screen.findByRole("button", { name: "+ Thêm học sinh" }));
    const wizard = await screen.findByRole("dialog", { name: /Thêm học sinh/ });
    expect(within(wizard).getByText("Bước 1/2")).toBeInTheDocument();
    await userEvent.type(within(wizard).getByRole("textbox", { name: "Họ và tên" }), "Lê Thu Hà");
    await userEvent.click(await within(wizard).findByRole("option", { name: /Nguyễn Thị Lan/ }));
    await userEvent.click(within(wizard).getByRole("button", { name: "Tiếp tục: Ghi danh →" }));

    const stepTwo = await screen.findByRole("dialog", { name: /Ghi danh vào lớp/ });
    expect(within(stepTwo).getByText("Bước 2/2")).toBeInTheDocument();
    await userEvent.click(within(stepTwo).getByRole("button", { name: "Để sau" }));

    expect(
      await screen.findByText('Đã lưu hồ sơ — ghi danh sau ở tab "Chưa vào lớp"'),
    ).toBeInTheDocument();
    expect(within(pageTabs()).getByRole("tab", { name: /^Chưa vào lớp/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });
});

describe("StudentsPage class search", () => {
  it("hides the class search while five or fewer classes exist", async () => {
    renderStudentsPage("/students?tab=by-class");

    await screen.findByRole("tab", { name: "Toán 6A" });
    expect(screen.queryByRole("searchbox", { name: "Tìm lớp" })).not.toBeInTheDocument();
  });

  it("filters the class pills without touching the page tabs", async () => {
    useSixClasses();
    renderStudentsPage("/students?tab=by-class");

    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    expect(within(classPills()).getAllByRole("tab")).toHaveLength(6);

    await userEvent.type(search, "văn");
    expect(
      within(classPills())
        .getAllByRole("tab")
        .map((tab) => tab.textContent),
    ).toEqual(["Văn 6A", "Văn 8C"]);
    // The page-level tabs are a separate tablist and never filter away.
    expect(within(pageTabs()).getAllByRole("tab")).toHaveLength(4);

    await userEvent.clear(search);
    expect(within(classPills()).getAllByRole("tab")).toHaveLength(6);
  });

  it("notes when no class matches", async () => {
    useSixClasses();
    renderStudentsPage("/students?tab=by-class");

    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    await userEvent.type(search, "hoá 12");

    expect(within(classPills()).queryAllByRole("tab")).toHaveLength(0);
    expect(screen.getByText('Không có lớp nào khớp "hoá 12"')).toBeInTheDocument();
  });
});

/** A member-shaped `/centers/me` holding exactly `permissions`. */
function asMember(permissions: string[]) {
  server.use(
    http.get(`${API_URL}/centers/me`, () =>
      HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
    ),
  );
}

describe("StudentsPage permissions", () => {
  it("gives the owner every write action and the contacts tab", async () => {
    renderStudentsPage("/students?tab=unenrolled");

    const table = await screen.findByRole("table");
    const row = (await within(table).findByRole("link", { name: "Trần Minh Khôi" })).closest("tr")!;
    expect(within(row).getByRole("button", { name: "Ghi danh vào lớp" })).toBeInTheDocument();
    expect(within(row).getByRole("button", { name: "Sửa" })).toBeInTheDocument();
    expect(within(row).getByRole("button", { name: "Xoá" })).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: "Nguyễn Thị Lan" })).toHaveAttribute(
      "href",
      `/contacts/${studentOnlyChild.contact_id}`,
    );
    expect(screen.getByRole("button", { name: "+ Thêm học sinh" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Nhập từ Excel" })).toHaveAttribute(
      "href",
      "/students/import",
    );
    expect(within(pageTabs()).getByRole("tab", { name: "Người liên hệ" })).toBeInTheDocument();
    expect(
      screen.queryByText("Liên hệ chủ trung tâm để thêm hoặc sửa học sinh."),
    ).not.toBeInTheDocument();
  });

  it("shows a students.list-only teacher a read-only list", async () => {
    asMember(["students.list"]);
    renderStudentsPage("/students?tab=unenrolled");

    const table = await screen.findByRole("table");
    const row = (await within(table).findByRole("link", { name: "Trần Minh Khôi" })).closest("tr")!;
    expect(
      await screen.findByText("Liên hệ chủ trung tâm để thêm hoặc sửa học sinh."),
    ).toBeInTheDocument();
    expect(within(row).queryByRole("button")).not.toBeInTheDocument();
    // Without contacts.view_all the contact name is plain text, not a link
    // to a page that would redirect away.
    expect(within(row).getByText("Nguyễn Thị Lan")).toBeInTheDocument();
    expect(within(row).queryByRole("link", { name: "Nguyễn Thị Lan" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "+ Thêm học sinh" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Nhập từ Excel" })).not.toBeInTheDocument();
    expect(
      within(pageTabs())
        .getAllByRole("tab")
        .map((tab) => tab.textContent),
    ).toEqual(["Tất cả", "Theo lớp", "Chưa vào lớp (2)"]);
  });

  it("falls back to the all tab when ?tab=contacts is opened without contacts.view_all", async () => {
    asMember(["students.list"]);
    renderStudentsPage("/students?tab=contacts");

    await screen.findByRole("tablist", { name: "Khu vực" });
    expect(await within(pageTabs()).findByRole("tab", { name: "Tất cả (3)" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.queryByRole("list", { name: "Danh sách người liên hệ" })).not.toBeInTheDocument();
  });

  it("offers the Excel import to a member holding imports.run", async () => {
    asMember(["students.list", "imports.run"]);
    renderStudentsPage();

    expect(await screen.findByRole("link", { name: "Nhập từ Excel" })).toHaveAttribute(
      "href",
      "/students/import",
    );
    expect(screen.queryByRole("button", { name: "+ Thêm học sinh" })).not.toBeInTheDocument();
  });

  it("redirects a member without students.list to the dashboard without any roster request", async () => {
    asMember([]);
    const requested: string[] = [];
    const onRequest = ({ request }: { request: Request }) => {
      requested.push(request.url);
    };
    server.events.on("request:start", onRequest);
    try {
      signInAs(testPrimaryTeacher);
      renderWithProviders(<StudentsPage />, {
        route: "/students",
        path: "/students",
        extraRoutes: [{ path: "/", element: <div>Trang tổng quan</div> }],
      });

      expect(await screen.findByText("Trang tổng quan")).toBeInTheDocument();
      // The guard is a shell around the content component, so the roster
      // queries never mount — zero requests beyond /centers/me itself.
      expect(
        requested.filter(
          (url) =>
            url.includes("/classes") ||
            url.includes("/students") ||
            url.includes("/contacts") ||
            url.includes("/sessions") ||
            url.includes("/enrollments"),
        ),
      ).toEqual([]);
    } finally {
      server.events.removeListener("request:start", onRequest);
    }
  });
});

describe("StudentsPage load more", () => {
  it("loads the next page on demand and hides the button once every row is in", async () => {
    const store = getRosterStore();
    for (let index = store.students.length; index < 60; index++) {
      store.students.push({
        ...studentOnlyChild,
        id: `50000000-0000-4000-8000-${String(index).padStart(12, "0")}`,
        full_name: `Học sinh ${index}`,
      });
    }
    renderStudentsPage();

    const table = await screen.findByRole("table");
    // Plain DOM count: role queries over a hundred-row table are slow.
    const bodyRows = () => table.querySelectorAll("tbody tr").length;
    expect(await screen.findByText("Đang hiện 50 / 60")).toBeInTheDocument();
    expect(bodyRows()).toBe(50);
    expect(within(pageTabs()).getByRole("tab", { name: "Tất cả (60)" })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Xem thêm" }));
    await waitFor(() => expect(bodyRows()).toBe(60));
    expect(screen.queryByRole("button", { name: "Xem thêm" })).not.toBeInTheDocument();
  });
});
