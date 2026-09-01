import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { StudentsPage } from "../pages/students-page";
import {
  classWithSchedule,
  dayOfCurrentMonth,
  enrollmentActive,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
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
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("StudentsPage tabs", () => {
  it("defaults a bare URL to the classes tab: name, schedule, price, settings link", async () => {
    renderStudentsPage();

    const tabs = await screen.findByRole("tablist", { name: "Khu vực" });
    expect(within(tabs).getByRole("tab", { name: "Lớp học" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    const table = await screen.findByRole("table");
    expect(await within(table).findByText("Toán 6A")).toBeInTheDocument();
    // classSchedule: weekday 2 (T3) at 18:00; default_unit_price 150000.
    expect(within(table).getByText("T3 — 18:00")).toBeInTheDocument();
    expect(within(table).getByText("150.000 ₫/buổi")).toBeInTheDocument();
    expect(within(table).getByRole("link", { name: "⚙ Cài đặt" })).toHaveAttribute(
      "href",
      `/classes/${classWithSchedule.id}/settings`,
    );
  });

  it("switches panels when clicking the page tabs", async () => {
    renderStudentsPage();

    await screen.findByRole("tablist", { name: "Khu vực" });
    await userEvent.click(within(pageTabs()).getByRole("tab", { name: "Học sinh" }));
    // The students panel brings the class pill strip with it.
    expect(await screen.findByRole("tablist", { name: "Lớp" })).toBeInTheDocument();
    expect(within(pageTabs()).getByRole("tab", { name: "Học sinh" })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    await userEvent.click(within(pageTabs()).getByRole("tab", { name: "Lớp học" }));
    // Mobile card + desktop table both render the per-class settings link.
    expect(await screen.findAllByRole("link", { name: "⚙ Cài đặt" })).not.toHaveLength(0);
    expect(screen.queryByRole("tablist", { name: "Lớp" })).not.toBeInTheDocument();
  });

  it("opens the students tab for a legacy ?class_id= link without a tab param", async () => {
    renderStudentsPage(`/students?class_id=${classWithSchedule.id}`);

    const pill = await screen.findByRole("tab", { name: "Toán 6A" });
    expect(pill).toHaveAttribute("aria-selected", "true");
    expect(within(pageTabs()).getByRole("tab", { name: "Học sinh" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("lets an explicit ?tab= win over a class_id in the URL", async () => {
    renderStudentsPage(`/students?tab=classes&class_id=${classWithSchedule.id}`);

    const tabs = await screen.findByRole("tablist", { name: "Khu vực" });
    expect(within(tabs).getByRole("tab", { name: "Lớp học" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(await screen.findAllByRole("link", { name: "⚙ Cài đặt" })).not.toHaveLength(0);
  });

  it("falls back to the resolution rule on an unknown ?tab= value", async () => {
    renderStudentsPage("/students?tab=bogus");

    const tabs = await screen.findByRole("tablist", { name: "Khu vực" });
    expect(within(tabs).getByRole("tab", { name: "Lớp học" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("issues no roster queries on the classes tab", async () => {
    const requested: string[] = [];
    const onRequest = ({ request }: { request: Request }) => {
      requested.push(request.url);
    };
    server.events.on("request:start", onRequest);
    try {
      renderStudentsPage();
      const table = await screen.findByRole("table");
      await within(table).findByText("Toán 6A");
      // Only the classes list may load here: the students query is disabled
      // and no class is selected, so sessions/enrollments stay silent too.
      expect(
        requested.filter(
          (url) =>
            url.includes("/students") || url.includes("/sessions") || url.includes("/enrollments"),
        ),
      ).toEqual([]);
    } finally {
      server.events.removeListener("request:start", onRequest);
    }
  });

  it("creates a class from the classes tab", async () => {
    renderStudentsPage();

    await userEvent.click(await screen.findByRole("button", { name: "+ Tạo lớp mới" }));
    expect(await screen.findByRole("dialog", { name: "Tạo lớp mới" })).toBeInTheDocument();
  });

  it("offers class creation from the classes-tab empty state", async () => {
    server.use(http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([], listMeta(0)))));
    renderStudentsPage();

    expect(await screen.findByText("Chưa có lớp nào.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "+ Tạo lớp mới" })).toBeInTheDocument();
  });

  it("shows an error instead of the empty state when the class list fails", async () => {
    server.use(http.get(`${API_URL}/classes`, () => HttpResponse.error()));
    renderStudentsPage();

    expect(await screen.findByText("Không tải được danh sách lớp")).toBeInTheDocument();
    expect(screen.queryByText("Chưa có lớp nào.")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "+ Tạo lớp mới" })).not.toBeInTheDocument();
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
    renderStudentsPage("/students?tab=students");

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
    renderStudentsPage("/students?tab=students");

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
    renderStudentsPage("/students?tab=students");

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
    renderStudentsPage("/students?tab=students");

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
      expect(within(pageTabs()).getByRole("tab", { name: "Chưa ghi danh" })).toHaveAttribute(
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
    await userEvent.click(within(pageTabs()).getByRole("tab", { name: "Chưa ghi danh" }));
    const table = await screen.findByRole("table");
    const link = await within(table).findByRole("link", { name: "Trần Minh Khôi" });
    const row = link.closest("tr")!;
    // No enrollment for this tab: both NHẬP HỌC and BUỔI T{m} degrade to "—".
    expect(within(row).getAllByText("—")).toHaveLength(2);
  });
});

describe("StudentsPage roster flows", () => {
  it("opens the edit dialog from Sửa and the anonymize dialog from Xoá", async () => {
    renderStudentsPage("/students?tab=students");

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
    expect(within(pageTabs()).getByRole("tab", { name: "Học sinh" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("postponing step 2 of the add-student wizard opens the unenrolled tab", async () => {
    renderStudentsPage("/students?tab=students");

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
      await screen.findByText('Đã lưu hồ sơ — ghi danh sau ở tab "Chưa ghi danh"'),
    ).toBeInTheDocument();
    expect(within(pageTabs()).getByRole("tab", { name: "Chưa ghi danh" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });
});

describe("StudentsPage class search", () => {
  it("hides the class search while five or fewer classes exist", async () => {
    renderStudentsPage("/students?tab=students");

    await screen.findByRole("tab", { name: "Toán 6A" });
    expect(screen.queryByRole("searchbox", { name: "Tìm lớp" })).not.toBeInTheDocument();
  });

  it("filters the class pills without touching the page tabs", async () => {
    useSixClasses();
    renderStudentsPage("/students?tab=students");

    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    expect(within(classPills()).getAllByRole("tab")).toHaveLength(6);

    await userEvent.type(search, "văn");
    expect(
      within(classPills())
        .getAllByRole("tab")
        .map((tab) => tab.textContent),
    ).toEqual(["Văn 6A", "Văn 8C"]);
    // The page-level tabs are a separate tablist and never filter away.
    expect(within(pageTabs()).getAllByRole("tab")).toHaveLength(3);

    await userEvent.clear(search);
    expect(within(classPills()).getAllByRole("tab")).toHaveLength(6);
  });

  it("notes when no class matches", async () => {
    useSixClasses();
    renderStudentsPage("/students?tab=students");

    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    await userEvent.type(search, "hoá 12");

    expect(within(classPills()).queryAllByRole("tab")).toHaveLength(0);
    expect(screen.getByText('Không có lớp nào khớp "hoá 12"')).toBeInTheDocument();
  });
});

describe("StudentsPage owner guard", () => {
  it("redirects a non-owner member to the dashboard without any roster request", async () => {
    server.use(
      // Member-shaped `/centers/me` (no `members` array).
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
      ),
    );
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
            url.includes("/sessions") ||
            url.includes("/enrollments"),
        ),
      ).toEqual([]);
    } finally {
      server.events.removeListener("request:start", onRequest);
    }
  });
});
