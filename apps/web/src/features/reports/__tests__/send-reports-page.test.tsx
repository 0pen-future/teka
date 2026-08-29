import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { SendReportsPage } from "../pages/send-reports-page";

const teacherLan = { id: "d0000000-0000-4000-8000-000000000001", name: "Cô Lan" };
const teacherMinh = { id: "d0000000-0000-4000-8000-000000000002", name: "Thầy Minh" };

function makeReportPeriod(overrides: Record<string, unknown> = {}) {
  return {
    id: "d1000000-0000-4000-8000-000000000001",
    teacher_id: teacherLan.id,
    teacher_name: teacherLan.name,
    year: 2026,
    month: 8,
    period_start: "2026-08-01",
    period_end: "2026-08-31",
    status: "open",
    closed_at: null,
    ...overrides,
  };
}

/**
 * Newest-first (`sort=-period_start`), exactly as the oversight list endpoint
 * answers: Minh's August leads, then Lan's August and July.
 */
const fixturePeriods = [
  makeReportPeriod({
    id: "d1000000-0000-4000-8000-000000000003",
    teacher_id: teacherMinh.id,
    teacher_name: teacherMinh.name,
    status: "open",
  }),
  makeReportPeriod({ id: "d1000000-0000-4000-8000-000000000001" }),
  makeReportPeriod({
    id: "d1000000-0000-4000-8000-000000000002",
    month: 7,
    period_start: "2026-07-01",
    period_end: "2026-07-31",
    status: "closed",
    closed_at: "2026-08-01T08:00:00Z",
  }),
];

function usePeriodsHandler(items: unknown[]) {
  server.use(
    http.get(`${API_URL}/billing-periods`, () =>
      HttpResponse.json(ok(items, listMeta(items.length))),
    ),
  );
}

function renderReportsPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<SendReportsPage />, { route: "/reports", path: "/reports" });
}

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("SendReportsPage", () => {
  it("groups periods by teacher in first-appearance order and links each to the send page", async () => {
    usePeriodsHandler(fixturePeriods);
    renderReportsPage();

    // Minh owns the most recent period, so his group leads the page.
    const headings = await screen.findAllByText(/^(Cô Lan|Thầy Minh)$/);
    expect(headings.map((el) => el.textContent)).toEqual([teacherMinh.name, teacherLan.name]);

    // Lan's two periods sit in her group, newest first, each opening the
    // existing send page for that period.
    const lanGroup = headings[1]!.closest("div")!;
    const lanLinks = within(lanGroup).getAllByRole("link");
    expect(lanLinks.map((link) => link.getAttribute("href"))).toEqual([
      "/notifications/d1000000-0000-4000-8000-000000000001",
      "/notifications/d1000000-0000-4000-8000-000000000002",
    ]);
    expect(
      screen.getByRole("link", { name: "Gửi báo cáo tháng 7/2026 của Cô Lan" }),
    ).toBeInTheDocument();

    // Status badges: open → Đang mở, closed → Đã chốt.
    expect(screen.getAllByText("Đang mở")).toHaveLength(2);
    expect(screen.getAllByText("Đã chốt")).toHaveLength(1);
  });

  it("falls back to a generic teacher label when the row carries no teacher name", async () => {
    usePeriodsHandler([makeReportPeriod({ teacher_name: undefined })]);
    renderReportsPage();

    expect(await screen.findByText("Giáo viên")).toBeInTheDocument();
  });

  it("shows an empty state when the center has no periods", async () => {
    usePeriodsHandler([]);
    renderReportsPage();

    expect(await screen.findByText("Chưa có kỳ học phí nào trong trung tâm.")).toBeInTheDocument();
  });

  it("shows an error state when the list cannot load", async () => {
    server.use(
      http.get(`${API_URL}/billing-periods`, () =>
        HttpResponse.json(fail("INTERNAL", "boom"), { status: 500 }),
      ),
    );
    renderReportsPage();

    expect(await screen.findByText("Không tải được danh sách kỳ học phí.")).toBeInTheDocument();
  });
});
