import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import {
  API_URL,
  listMeta,
  makeClass,
  makeClassSession,
  makeCollectionsSummary,
  makePendingSession,
  makePeriod,
  makePreview,
  ok,
} from "@/test/msw/handlers";
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
  it("greets the signed-in teacher with a time-of-day salutation", async () => {
    renderDashboard();

    expect(
      await screen.findByRole("heading", {
        name: new RegExp(`^Chào buổi (sáng|trưa|chiều|tối), ${testPrimaryTeacher.full_name}!$`),
      }),
    ).toBeInTheDocument();
  });

  it("merges pending sessions into one banner whose action opens the first session", async () => {
    server.use(
      http.get(`${API_URL}/sessions/pending`, () =>
        HttpResponse.json(
          ok({
            total: 2,
            items: [
              makePendingSession({ session_id: "session-1", class_name: "Toán 6A" }),
              makePendingSession({
                session_id: "session-2",
                class_name: "Toán 9B",
                session_date: "2026-07-16",
              }),
            ],
          }),
        ),
      ),
    );
    renderDashboard();

    expect(await screen.findByText("Có 2 buổi đã dạy nhưng chưa điểm danh")).toBeInTheDocument();
    expect(
      screen.getByText(/Toán 6A — .* · Toán 9B — .*Chưa điểm danh là chưa tính được tiền\./),
    ).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Điểm danh ngay" });
    expect(link).toHaveAttribute("href", "/sessions/session-1/attendance");
  });

  it("renders no banner at all when there are no pending sessions", async () => {
    server.use(
      http.get(`${API_URL}/sessions/pending`, () => HttpResponse.json(ok({ total: 0, items: [] }))),
    );
    renderDashboard();

    // Settle on another async element so the absence check is not a race.
    await screen.findByText("HỌC SINH");
    expect(screen.queryByText("Điểm danh ngay")).not.toBeInTheDocument();
    expect(screen.queryByText(/buổi đã dạy nhưng chưa điểm danh/)).not.toBeInTheDocument();
  });

  it("surfaces a visible warning instead of a blank screen when the pending-sessions fetch fails", async () => {
    server.use(http.get(`${API_URL}/sessions/pending`, () => HttpResponse.error()));
    renderDashboard();

    expect(
      await screen.findByText("Không tải được danh sách buổi cần điểm danh"),
    ).toBeInTheDocument();
  });

  it("computes the four stats from roster, sessions, preview, and period state", async () => {
    const classA = makeClass({ id: "class-a", name: "Toán 9A" });
    const classB = makeClass({ id: "class-b", name: "Toán 6B" });
    server.use(
      http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([classA, classB], listMeta(2)))),
      http.get(`${API_URL}/students`, ({ request }) => {
        const classId = new URL(request.url).searchParams.get("class_id");
        const totals: Record<string, number> = { "class-a": 8, "class-b": 4 };
        return HttpResponse.json(ok([], listMeta(classId ? (totals[classId] ?? 0) : 12)));
      }),
      http.get(`${API_URL}/classes/:id/sessions`, ({ params }) =>
        HttpResponse.json(
          ok(
            params.id === "class-a"
              ? [
                  makeClassSession({ id: "s1", class_id: "class-a" }),
                  makeClassSession({
                    id: "s2",
                    class_id: "class-a",
                    status: "planned",
                    attendance_confirmed_at: null,
                  }),
                  makeClassSession({
                    id: "s3",
                    class_id: "class-a",
                    status: "cancelled",
                    attendance_confirmed_at: null,
                  }),
                ]
              : [makeClassSession({ id: "s4", class_id: "class-b" })],
          ),
        ),
      ),
      http.get(`${API_URL}/billing-periods/:id/preview`, () =>
        HttpResponse.json(
          ok(makePreview({ totals: { ...makePreview().totals, total_due: 3600000 } })),
        ),
      ),
    );
    renderDashboard();

    expect(await screen.findByText("12")).toBeInTheDocument();
    expect(screen.getByText("2 lớp đang chạy")).toBeInTheDocument();
    // class-a: s1 confirmed, s2 planned, s3 cancelled (excluded);
    // class-b: s4 confirmed → 2 of 3 countable sessions confirmed = 67%.
    expect(await screen.findByText("67%")).toBeInTheDocument();
    expect(screen.getByText("2/3 buổi đã xác nhận")).toBeInTheDocument();
    expect(await screen.findByText("3.600.000 ₫")).toBeInTheDocument();
    expect(screen.getByText("Chưa chốt sổ")).toBeInTheDocument();
    // Open period → collections have not started.
    expect(screen.getByText("—")).toBeInTheDocument();
    expect(screen.getByText("Chốt sổ để bắt đầu thu")).toBeInTheDocument();
  });

  it("shows collected totals once the period is closed", async () => {
    server.use(
      http.post(`${API_URL}/billing-periods`, () =>
        HttpResponse.json(ok(makePeriod({ status: "closed" })), { status: 201 }),
      ),
      http.get(`${API_URL}/billing-periods/:id/collections/summary`, () =>
        HttpResponse.json(
          ok(makeCollectionsSummary({ total_paid: 2000000, total_outstanding: 500000 })),
        ),
      ),
    );
    renderDashboard();

    expect(await screen.findByText("2.000.000 ₫")).toBeInTheDocument();
    expect(screen.getByText("còn 500.000 ₫")).toBeInTheDocument();
    expect(await screen.findByText("Đã chốt sổ")).toBeInTheDocument();
  });

  it("renders one card per class linking to its attendance screen", async () => {
    const classA = makeClass({ id: "class-a", name: "Toán 9A" });
    server.use(
      http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([classA], listMeta(1)))),
      http.get(`${API_URL}/students`, () => HttpResponse.json(ok([], listMeta(8)))),
      http.get(`${API_URL}/classes/:id/sessions`, () =>
        HttpResponse.json(
          ok([
            makeClassSession({ id: "s1", class_id: "class-a" }),
            makeClassSession({
              id: "s2",
              class_id: "class-a",
              status: "planned",
              attendance_confirmed_at: null,
            }),
          ]),
        ),
      ),
    );
    renderDashboard();

    const card = (await screen.findByText("Toán 9A")).closest("a");
    expect(card).not.toBeNull();
    expect(card).toHaveAttribute("href", "/sessions?class_id=class-a");
    const scope = within(card as HTMLElement);
    expect(await scope.findByText("Thiếu 1")).toBeInTheDocument();
    expect(scope.getByText("1/2")).toBeInTheDocument();
    expect(scope.getByText("8")).toBeInTheDocument();
    expect(scope.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "50");
  });

  it("marks the period-backed stats as failed instead of loading forever when the period fetch fails", async () => {
    server.use(http.post(`${API_URL}/billing-periods`, () => HttpResponse.error()));
    renderDashboard();

    // Attendance, due, and collected all depend on the period.
    expect(await screen.findAllByText("Không tải được")).toHaveLength(3);
  });

  it("flags a class card whose sessions fetch fails instead of showing a fake 0/0", async () => {
    const classA = makeClass({ id: "class-a", name: "Toán 9A" });
    server.use(
      http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([classA], listMeta(1)))),
      http.get(`${API_URL}/classes/:id/sessions`, () => HttpResponse.error()),
    );
    renderDashboard();

    const card = (await screen.findByText("Toán 9A")).closest("a");
    expect(
      await within(card as HTMLElement).findByText("Không tải được buổi học"),
    ).toBeInTheDocument();
    expect(within(card as HTMLElement).queryByText(/buổi đã điểm danh/)).not.toBeInTheDocument();
  });

  it("labels a class without sessions 'Lớp mới' and links it to the roster", async () => {
    const classA = makeClass({ id: "class-new", name: "Toán 7C" });
    server.use(http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([classA], listMeta(1)))));
    renderDashboard();

    const card = (await screen.findByText("Toán 7C")).closest("a");
    expect(card).toHaveAttribute("href", "/students?class_id=class-new");
    expect(await within(card as HTMLElement).findByText("Lớp mới")).toBeInTheDocument();
  });

  it("hides the sessionless class's card from a non-owner member", async () => {
    const classNew = makeClass({ id: "class-new", name: "Toán 7C" });
    const classOld = makeClass({ id: "class-old", name: "Toán 9A" });
    server.use(
      // Member-shaped `/centers/me` (no `members` array).
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
      ),
      http.get(`${API_URL}/classes`, () =>
        HttpResponse.json(ok([classNew, classOld], listMeta(2))),
      ),
      http.get(`${API_URL}/classes/:id/sessions`, ({ params }) =>
        HttpResponse.json(
          ok(
            params.id === "class-old"
              ? [makeClassSession({ id: "s1", class_id: "class-old" })]
              : [],
          ),
        ),
      ),
    );
    renderDashboard();

    // The class with sessions keeps its attendance card…
    expect((await screen.findByText("Toán 9A")).closest("a")).toHaveAttribute(
      "href",
      "/sessions?class_id=class-old",
    );
    // …while the sessionless card would link the owner-only roster page and
    // bounce a member straight back here, so it never renders for them.
    expect(screen.queryByText("Toán 7C")).not.toBeInTheDocument();
    expect(screen.queryByText("Lớp mới")).not.toBeInTheDocument();
  });
});
