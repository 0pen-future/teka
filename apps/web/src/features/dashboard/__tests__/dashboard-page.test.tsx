import { screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, makePendingSession, makePeriod, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { DashboardPage } from "../pages/dashboard-page";

function renderDashboard() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<DashboardPage />);
}

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("DashboardPage", () => {
  it("greets the signed-in teacher", async () => {
    renderDashboard();

    expect(await screen.findByText(`Chào ${testPrimaryTeacher.full_name} 👋`)).toBeInTheDocument();
  });

  it("lists each pending session with a link to its attendance screen", async () => {
    server.use(
      http.get(`${API_URL}/sessions/pending`, () =>
        HttpResponse.json(
          ok({
            total: 1,
            items: [makePendingSession({ session_id: "session-1", class_name: "Toán 6A" })],
          }),
        ),
      ),
    );
    renderDashboard();

    expect(await screen.findByText(/Toán 6A/)).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Điểm danh ngay" });
    expect(link).toHaveAttribute("href", "/sessions/session-1/attendance");
  });

  it("shows a quiet success line when there are no pending sessions", async () => {
    server.use(
      http.get(`${API_URL}/sessions/pending`, () => HttpResponse.json(ok({ total: 0, items: [] }))),
    );
    renderDashboard();

    expect(await screen.findByText("Đã điểm danh đủ các buổi đã qua")).toBeInTheDocument();
    expect(screen.queryByText("Điểm danh ngay")).not.toBeInTheDocument();
  });

  it("shows the close-period action when the current period is open", async () => {
    server.use(
      http.post(`${API_URL}/billing-periods`, () =>
        HttpResponse.json(ok(makePeriod({ status: "open" })), { status: 201 }),
      ),
    );
    renderDashboard();

    const link = await screen.findByRole("link", { name: "Chốt sổ" });
    expect(link).toHaveAttribute("href", "/billing/30000000-0000-4000-8000-000000000001");
  });

  it("shows the collections action when the current period is closed", async () => {
    server.use(
      http.post(`${API_URL}/billing-periods`, () =>
        HttpResponse.json(ok(makePeriod({ status: "closed" })), { status: 201 }),
      ),
    );
    renderDashboard();

    const link = await screen.findByRole("link", { name: "Xem thu tiền" });
    expect(link).toHaveAttribute("href", "/collections/30000000-0000-4000-8000-000000000001");
  });

  it("surfaces a visible warning instead of a blank screen when the pending-sessions fetch fails", async () => {
    server.use(http.get(`${API_URL}/sessions/pending`, () => HttpResponse.error()));
    renderDashboard();

    expect(
      await screen.findByText("Không tải được danh sách buổi cần điểm danh"),
    ).toBeInTheDocument();
  });

  it("surfaces a visible warning instead of a blank card when the current-period fetch fails", async () => {
    server.use(http.post(`${API_URL}/billing-periods`, () => HttpResponse.error()));
    renderDashboard();

    expect(await screen.findByText("Không tải được kỳ hiện tại")).toBeInTheDocument();
  });
});
