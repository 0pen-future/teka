import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassDetailPage } from "../pages/class-detail-page";
import {
  classWithSchedule,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
} from "./roster-handlers";

function renderPage(route = `/classes/${classWithSchedule.id}`) {
  return renderWithProviders(<ClassDetailPage />, {
    route,
    path: "/classes/:id",
    extraRoutes: [
      { path: "/classes", element: <div>class-list-stub</div> },
      { path: "/classes/:id/settings", element: <div>class-settings-stub</div> },
    ],
  });
}

/** Non-owner center member without any class staff role: read-only everywhere. */
const memberCenterHandler = http.get(`${API_URL}/centers/me`, () =>
  HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions: ["classes.read"] })),
);

const todayIso = () => new Date().toISOString().slice(0, 10);

function addDays(date: string, days: number): string {
  const d = new Date(`${date}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

describe("ClassDetailPage", () => {
  beforeEach(() => {
    resetRosterStore();
    server.use(...rosterHandlers);
    signInAs(testPrimaryTeacher);
  });

  afterEach(() => {
    useAuthStore.getState().clearSession();
  });

  it("renders the header with code, phase chip, back link and settings link", async () => {
    renderPage();
    expect(await screen.findByRole("heading", { name: "Toán 6A" })).toBeInTheDocument();
    expect(screen.getByText("Mã lớp: TOAN6A")).toBeInTheDocument();
    expect(screen.getByText("Đang học")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Danh sách lớp học" })).toHaveAttribute(
      "href",
      "/classes",
    );
    expect(screen.getByRole("link", { name: "Sửa lớp" })).toHaveAttribute(
      "href",
      `/classes/${classWithSchedule.id}/settings`,
    );
  });

  it("links the course chip to the catalog when the class is attached to a course", async () => {
    getRosterStore().classes[0]!.course = {
      id: "90000000-0000-4000-8000-000000000001",
      code: "TOAN-6",
      name: "Toán 6 nền tảng",
    };
    renderPage();
    expect(await screen.findByRole("heading", { name: "Toán 6A" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Khóa: TOAN-6 · Toán 6 nền tảng" })).toHaveAttribute(
      "href",
      "/courses/90000000-0000-4000-8000-000000000001",
    );
  });

  it("shows the course as plain text when the member cannot open the catalog", async () => {
    server.use(memberCenterHandler);
    getRosterStore().classes[0]!.course = {
      id: "90000000-0000-4000-8000-000000000001",
      code: "TOAN-6",
      name: "Toán 6 nền tảng",
    };
    renderPage();
    expect(await screen.findByRole("heading", { name: "Toán 6A" })).toBeInTheDocument();
    expect(screen.getByText("Khóa: TOAN-6 · Toán 6 nền tảng")).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Khóa: TOAN-6 · Toán 6 nền tảng" }),
    ).not.toBeInTheDocument();
  });

  it("shows the not-found block for an unknown class", async () => {
    server.use(
      http.get(`${API_URL}/classes/:id`, () =>
        HttpResponse.json(fail("NOT_FOUND", "class not found"), { status: 404 }),
      ),
    );
    renderPage("/classes/00000000-0000-4000-8000-000000000404");
    expect(await screen.findByRole("alert")).toHaveTextContent("Không tìm thấy lớp");
  });

  it("copies the class code to the clipboard and confirms with a toast", async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6A" });

    await user.click(screen.getByRole("button", { name: "Sao chép" }));

    expect(writeText).toHaveBeenCalledWith("TOAN6A");
    expect(await screen.findByText("Đã sao chép mã lớp")).toBeInTheDocument();
  });

  it("opens on the Thông tin tab with every tab enabled", async () => {
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6A" });
    for (const name of ["Thông tin", "Học viên", "Buổi học", "Chat", "Bài tập", "Tài liệu"]) {
      expect(screen.getByRole("tab", { name })).toBeEnabled();
    }
    expect(screen.getByRole("tab", { name: "Thông tin" })).toHaveAttribute("aria-selected", "true");
  });

  describe("Thông tin tab", () => {
    it("shows the weekly schedule, the ops card and the shortcut cards", async () => {
      renderPage();
      await screen.findByRole("heading", { name: "Toán 6A" });

      expect(screen.getByText("Lịch học hàng tuần")).toBeInTheDocument();
      expect(screen.getByText("Tối Thứ Ba")).toBeInTheDocument();
      expect(screen.getByText("Thông tin vận hành")).toBeInTheDocument();
      expect(screen.getByRole("switch", { name: "Cần tuyển sinh" })).not.toBeChecked();
      expect(screen.getByText("Khối 6")).toBeInTheDocument();

      expect(screen.getByRole("link", { name: "Sổ đầu bài" })).toHaveAttribute(
        "href",
        `/classbook?class_id=${classWithSchedule.id}`,
      );
      expect(screen.getByRole("link", { name: "Hồ sơ học sinh" })).toHaveAttribute(
        "href",
        `/records?class_id=${classWithSchedule.id}`,
      );
      expect(screen.getByRole("link", { name: "Điểm danh" })).toHaveAttribute("href", "/sessions");
      expect(screen.getByRole("link", { name: "Thiết lập lớp" })).toHaveAttribute(
        "href",
        `/classes/${classWithSchedule.id}/settings`,
      );
      expect(screen.getByRole("link", { name: "Cấu hình lớp học" })).toHaveAttribute(
        "href",
        "/center/class-config",
      );

      expect(screen.getByText("Lịch sử lớp")).toBeInTheDocument();
      expect(screen.getByText("Chương trình học")).toBeInTheDocument();
      expect(await screen.findByText("Lớp chưa áp dụng chương trình mẫu.")).toBeInTheDocument();
    });

    it("lists the teaching staff and the hoc_vu staff as the class contact", async () => {
      getRosterStore().classStaff.push({
        id: "90000000-0000-4000-8000-000000000002",
        class_id: classWithSchedule.id,
        teacher_id: "73000000-0000-4000-8000-000000000002",
        teacher_name: "Cô Hoa",
        role_key: "hoc_vu",
        role_label: "Học vụ",
        started_at: "2026-01-01T08:00:00Z",
        ended_at: null,
      });
      renderPage();
      const team = await screen.findByRole("region", { name: "Đội ngũ giảng dạy" });
      expect(await within(team).findByText("Cô Lan")).toBeInTheDocument();
      expect(within(team).getByText("Cô Hoa")).toBeInTheDocument();
      expect(within(team).getByRole("button", { name: "+ Mời GV" })).toBeEnabled();

      const contact = screen.getByRole("region", { name: "Nhân viên phụ trách" });
      expect(within(contact).getByText("Cô Hoa")).toBeInTheDocument();
      expect(within(contact).queryByText("Cô Lan")).not.toBeInTheDocument();
    });

    it("toggling Cần tuyển sinh sends only the changed catalog field", async () => {
      const user = userEvent.setup();
      let body: Record<string, unknown> | undefined;
      server.use(
        http.put(`${API_URL}/classes/:id`, async ({ request }) => {
          body = (await request.json()) as Record<string, unknown>;
          return HttpResponse.json(ok({ ...classWithSchedule, recruiting: true }));
        }),
      );
      renderPage();
      const toggle = await screen.findByRole("switch", { name: "Cần tuyển sinh" });

      await user.click(toggle);

      await waitFor(() => {
        expect(body).toEqual({
          name: "Toán 6A",
          start_date: "2026-01-05",
          end_date: "",
          default_unit_price: 150000,
          recruiting: true,
        });
      });
      expect(await screen.findByText("Đã cập nhật lớp")).toBeInTheDocument();
    });

    it("saves an edited note and tags through the same partial update", async () => {
      const user = userEvent.setup();
      let body: Record<string, unknown> | undefined;
      server.use(
        http.put(`${API_URL}/classes/:id`, async ({ request }) => {
          body = (await request.json()) as Record<string, unknown>;
          return HttpResponse.json(
            ok({ ...classWithSchedule, note: "Phòng 201", tags: ["Toán", "Khối 6", "Nâng cao"] }),
          );
        }),
      );
      renderPage();
      await screen.findByText("Thông tin vận hành");

      await user.click(screen.getByRole("button", { name: "Sửa thông tin vận hành" }));
      await user.type(screen.getByLabelText("Ghi chú"), "Phòng 201");
      await user.type(screen.getByLabelText("Thêm thẻ"), "Nâng cao{enter}");
      await user.click(screen.getByRole("button", { name: "Lưu" }));

      await waitFor(() => {
        expect(body).toMatchObject({ note: "Phòng 201", tags: ["Toán", "Khối 6", "Nâng cao"] });
      });
      expect(body).not.toHaveProperty("recruiting");
    });

    it("hides the ops controls from a member who cannot write the class", async () => {
      server.use(memberCenterHandler);
      renderPage();
      await screen.findByText("Thông tin vận hành");
      await screen.findByText("Lịch học hàng tuần");
      expect(screen.queryByRole("switch", { name: "Cần tuyển sinh" })).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "Sửa thông tin vận hành" }),
      ).not.toBeInTheDocument();
      expect(screen.queryByRole("link", { name: "Cấu hình lớp học" })).not.toBeInTheDocument();
    });
  });

  describe("Học viên tab", () => {
    it("lists the active enrollments with the contact and start date", async () => {
      renderPage(`/classes/${classWithSchedule.id}?tab=students`);
      const table = await screen.findByRole("table");
      expect(screen.getByRole("tab", { name: "Học viên" })).toHaveAttribute(
        "aria-selected",
        "true",
      );
      expect(screen.getByText("Học viên (1)")).toBeInTheDocument();
      expect(within(table).getByText("Nguyễn Văn An")).toBeInTheDocument();
      expect(within(table).getByText("05/01/2026")).toBeInTheDocument();
      expect(screen.getByRole("link", { name: "Thêm học viên" })).toHaveAttribute(
        "href",
        `/students?class_id=${classWithSchedule.id}`,
      );
    });

    it("tells the reader when the roster was cut to the first page", async () => {
      server.use(
        http.get(`${API_URL}/enrollments`, () =>
          HttpResponse.json(ok(getRosterStore().enrollments, listMeta(120, 1, 100))),
        ),
      );
      renderPage(`/classes/${classWithSchedule.id}?tab=students`);
      await screen.findByRole("table");
      const notice = screen.getByRole("note");
      expect(notice).toHaveTextContent("Đang hiển thị 1/120 học viên.");
      expect(within(notice).getByRole("link", { name: "Xem tất cả" })).toHaveAttribute(
        "href",
        `/students?class_id=${classWithSchedule.id}`,
      );
    });

    it("shows the recruiting hint when the class has no student", async () => {
      getRosterStore().enrollments.length = 0;
      renderPage(`/classes/${classWithSchedule.id}?tab=students`);
      expect(
        await screen.findByText(
          "Lớp chưa có học viên. Bật “Cần tuyển sinh” để đưa lớp vào danh sách tuyển sinh.",
        ),
      ).toBeInTheDocument();
    });
  });

  describe("Buổi học tab", () => {
    it("queries the class window from start_date to end_date and labels the source", async () => {
      const seen: URLSearchParams[] = [];
      server.use(
        http.get(`${API_URL}/classes/:classId/sessions`, ({ request }) => {
          seen.push(new URL(request.url).searchParams);
          return HttpResponse.json(ok(getRosterStore().sessions));
        }),
        http.get(`${API_URL}/classes/:id`, () =>
          HttpResponse.json(ok({ ...classWithSchedule, end_date: "2026-12-20" })),
        ),
      );
      renderPage(`/classes/${classWithSchedule.id}?tab=sessions`);
      const table = await screen.findByRole("table");

      expect(seen).toHaveLength(1);
      expect(seen[0]!.get("from")).toBe("2026-01-05");
      expect(seen[0]!.get("to")).toBe("2026-12-20");
      // Browsing must never materialise sessions.
      expect(seen[0]!.get("readonly")).toBe("true");

      const rows = within(table).getAllByRole("row").slice(1);
      expect(rows).toHaveLength(5);
      // The fixture sessions all start 18:00 on the weekly slot's weekday
      // only when the date falls on a Tuesday; the rest were added by hand.
      const sources = rows.map((row) => within(row).getAllByRole("cell").at(-1)!.textContent);
      const sessions = getRosterStore().sessions;
      sessions.forEach((session, index) => {
        const weekday = new Date(`${session.session_date}T00:00:00Z`).getUTCDay();
        expect(sources[index]).toBe(weekday === 2 ? "Lịch tuần" : "Thêm tay");
      });
      expect(screen.getByRole("link", { name: "Điểm danh & nhận xét →" })).toHaveAttribute(
        "href",
        `/classbook?class_id=${classWithSchedule.id}`,
      );
    });

    it("caps an open-ended class at 90 days ahead and shows the empty copy", async () => {
      const seen: URLSearchParams[] = [];
      server.use(
        http.get(`${API_URL}/classes/:classId/sessions`, ({ request }) => {
          seen.push(new URL(request.url).searchParams);
          return HttpResponse.json(ok([]));
        }),
      );
      renderPage(`/classes/${classWithSchedule.id}?tab=sessions`);
      expect(
        await screen.findByText(
          "Chưa có buổi học. Áp dụng chương trình mẫu để sinh danh sách buổi theo lịch hàng tuần.",
        ),
      ).toBeInTheDocument();
      expect(seen[0]!.get("from")).toBe("2026-01-05");
      expect(seen[0]!.get("to")).toBe(addDays(todayIso(), 90));
      expect(seen[0]!.get("readonly")).toBe("true");
    });

    it("spans a class longer than the generation cap in one read-only request", async () => {
      const seen: URLSearchParams[] = [];
      server.use(
        http.get(`${API_URL}/classes/:classId/sessions`, ({ request }) => {
          seen.push(new URL(request.url).searchParams);
          return HttpResponse.json(ok([]));
        }),
        http.get(`${API_URL}/classes/:id`, () =>
          HttpResponse.json(ok({ ...classWithSchedule, end_date: "2028-06-30" })),
        ),
      );
      renderPage(`/classes/${classWithSchedule.id}?tab=sessions`);
      await screen.findByText(
        "Chưa có buổi học. Áp dụng chương trình mẫu để sinh danh sách buổi theo lịch hàng tuần.",
      );
      expect(seen).toHaveLength(1);
      expect(seen[0]!.get("to")).toBe("2028-06-30");
      expect(seen[0]!.get("readonly")).toBe("true");
    });
  });
});
