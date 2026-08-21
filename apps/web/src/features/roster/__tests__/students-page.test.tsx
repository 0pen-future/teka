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

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
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

    await screen.findByRole("tab", { name: "Toán 6A" });
    expect(
      screen.getByText(
        "Chỉ lưu: họ tên · ngày nhập học · lớp · người liên hệ. Không thu thập gì thêm.",
      ),
    ).toBeInTheDocument();
  });

  it("renders the v2 columns with the current month in the sessions header", async () => {
    renderStudentsPage();

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

  it("links ⚙ Cài đặt lớp to the selected class's settings", async () => {
    renderStudentsPage();

    expect(await screen.findByRole("link", { name: "⚙ Cài đặt lớp" })).toHaveAttribute(
      "href",
      `/classes/${classWithSchedule.id}/settings`,
    );
  });

  it("shows enrollment start and this month's non-cancelled session count", async () => {
    renderStudentsPage();

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
    renderStudentsPage();

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
      renderStudentsPage("/students?class_id=none");
      await screen.findAllByRole("link", { name: "Trần Minh Khôi" });
      // No class is selected: neither the sessions nor the enrollments
      // query may fire (acceptance criterion 3 — no hidden fan-out).
      expect(
        requested.filter((url) => url.includes("/sessions") || url.includes("/enrollments")),
      ).toEqual([]);
    } finally {
      server.events.removeListener("request:start", onRequest);
    }
  });

  it("renders dashes and no settings link on the unenrolled tab", async () => {
    renderStudentsPage();

    await userEvent.click(await screen.findByRole("tab", { name: "Chưa ghi danh" }));
    const table = screen.getByRole("table");
    const link = await within(table).findByRole("link", { name: "Trần Minh Khôi" });
    const row = link.closest("tr")!;
    // No enrollment for this tab: both NHẬP HỌC and BUỔI T{m} degrade to "—".
    expect(within(row).getAllByText("—")).toHaveLength(2);
    expect(screen.queryByRole("link", { name: "⚙ Cài đặt lớp" })).not.toBeInTheDocument();
  });
});

describe("StudentsPage class search", () => {
  it("hides the class search while five or fewer classes exist", async () => {
    renderStudentsPage();

    await screen.findByRole("tab", { name: "Toán 6A" });
    expect(screen.queryByRole("searchbox", { name: "Tìm lớp" })).not.toBeInTheDocument();
  });

  it("filters only real class tabs, keeping the unenrolled tab", async () => {
    useSixClasses();
    renderStudentsPage();

    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    expect(await screen.findAllByRole("tab")).toHaveLength(7);

    await userEvent.type(search, "văn");
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "Văn 6A",
      "Văn 8C",
      "Chưa ghi danh",
    ]);

    await userEvent.clear(search);
    expect(screen.getAllByRole("tab")).toHaveLength(7);
  });

  it("notes when no class matches while the unenrolled tab stays", async () => {
    useSixClasses();
    renderStudentsPage();

    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    await userEvent.type(search, "hoá 12");

    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["Chưa ghi danh"]);
    expect(screen.getByText('Không có lớp nào khớp "hoá 12"')).toBeInTheDocument();
  });
});
