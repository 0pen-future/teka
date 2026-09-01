import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import type { Class } from "@/features/roster";
import { formatSessionDate } from "@/lib/utils";
import { API_URL, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { SessionsPage } from "../pages/sessions-page";
import type { Session } from "../schemas/attendance-schemas";
import {
  attendanceHandlers,
  fixtureClass,
  resetAttendanceStore,
  sessionConfirmed,
  sessionUnconfirmedPast,
  sessionUpcoming,
} from "./attendance-handlers";

/** Six active classes — one past the threshold that reveals "Tìm lớp…". */
function sixClasses(): Class[] {
  return [
    fixtureClass,
    ...["Toán 7B", "Văn 6A", "Văn 8C", "Anh 9A", "Lý 8B"].map((name, index) => ({
      ...fixtureClass,
      id: `90000000-0000-4000-8000-00000000001${index}`,
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

function renderSessionsPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<SessionsPage />, { route: "/sessions", path: "/sessions" });
}

/**
 * The trio's arrows and the calendar navigate to `/sessions/:id/attendance`;
 * a flat stub route lets tests assert the target pathname without mounting
 * the real attendance panel.
 */
function renderSessionsPageWithAttendanceStub() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<SessionsPage />, {
    route: "/sessions",
    path: "/sessions",
    extraRoutes: [{ path: "/sessions/:id/attendance", element: <div /> }],
  });
}

beforeEach(() => {
  resetAttendanceStore();
  server.use(...attendanceHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("SessionsPage", () => {
  it("groups the unconfirmed past session under a dedicated heading, flagged first", async () => {
    renderSessionsPage();

    const heading = await screen.findByRole("heading", { name: "Cần điểm danh" });
    expect(heading).toBeInTheDocument();

    const flaggedRow = await screen.findByText(
      `${sessionUnconfirmedPast.class_name} — ${formatSessionDate(sessionUnconfirmedPast.session_date)}`,
    );
    expect(flaggedRow.closest("a")).toHaveTextContent("Chưa điểm danh");
  });

  it("shows the class as a selectable pill tab", async () => {
    renderSessionsPage();

    const tab = await screen.findByRole("tab", { name: "Toán 6A" });
    expect(tab).toHaveAttribute("aria-selected", "true");
  });

  it("hides the class search while five or fewer classes exist", async () => {
    renderSessionsPage();

    await screen.findByRole("tab", { name: "Toán 6A" });
    expect(screen.queryByRole("searchbox", { name: "Tìm lớp" })).not.toBeInTheDocument();
  });

  it("filters class tabs by name once more than five classes exist", async () => {
    useSixClasses();
    renderSessionsPage();

    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    expect(await screen.findAllByRole("tab")).toHaveLength(6);

    await userEvent.type(search, "văn");
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["Văn 6A", "Văn 8C"]);
    // Filtering only narrows the pills — the selected class is untouched.
    expect(screen.getByRole("heading", { name: "Cần điểm danh" })).toBeInTheDocument();

    await userEvent.clear(search);
    expect(screen.getAllByRole("tab")).toHaveLength(6);
  });

  it("notes when no class matches the search", async () => {
    useSixClasses();
    renderSessionsPage();

    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    await userEvent.type(search, "hoá 12");

    expect(screen.queryAllByRole("tab")).toHaveLength(0);
    expect(screen.getByText('Không có lớp nào khớp "hoá 12"')).toBeInTheDocument();
  });

  it("selects the class named by ?class_id= (dashboard deep link)", async () => {
    useSixClasses();
    signInAs(testPrimaryTeacher);
    renderWithProviders(<SessionsPage />, {
      route: "/sessions?class_id=90000000-0000-4000-8000-000000000011",
      path: "/sessions",
    });

    expect(await screen.findByRole("tab", { name: "Văn 6A" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("ignores a ?class_id= not in the active list and falls back to the first class", async () => {
    useSixClasses();
    signInAs(testPrimaryTeacher);
    renderWithProviders(<SessionsPage />, {
      route: "/sessions?class_id=ffffffff-0000-4000-8000-000000000000",
      path: "/sessions",
    });

    expect(await screen.findByRole("tab", { name: fixtureClass.name })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("anchors the trio to the nearest upcoming session when today has none", async () => {
    renderSessionsPage();

    // No fixture session falls on today, so the anchor is the nearest
    // upcoming one and the center caption reads ĐANG XEM, never HÔM NAY.
    expect(await screen.findByText("ĐANG XEM")).toBeInTheDocument();
    expect(screen.queryByText("HÔM NAY")).not.toBeInTheDocument();

    const centerCard = screen.getByRole("link", {
      name: new RegExp(formatSessionDate(sessionUpcoming.session_date)),
    });
    expect(centerCard).toHaveTextContent("Sắp tới");

    // The previous slot shows the confirmed session with its summary badge.
    const prevCard = screen.getByRole("link", {
      name: new RegExp(formatSessionDate(sessionConfirmed.session_date)),
    });
    expect(prevCard).toHaveTextContent("Đã điểm danh");
    expect(prevCard).toHaveTextContent("27 đúng giờ · 1 muộn · 1 vắng · 1 có lý do");

    // The anchor is the last session in the window: forward is a boundary.
    expect(screen.getByRole("button", { name: "Buổi kế tiếp" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Buổi trước" })).toBeEnabled();
    expect(screen.getByText("Chưa có buổi")).toBeInTheDocument();
  });

  it("anchors the trio to today's session when one exists", async () => {
    const sessionToday: Session = {
      ...sessionUnconfirmedPast,
      id: "91000000-0000-4000-8000-000000000009",
      session_date: new Date().toISOString().slice(0, 10),
    };
    server.use(
      http.get(`${API_URL}/classes/:classId/sessions`, () =>
        HttpResponse.json(
          ok([sessionUnconfirmedPast, sessionConfirmed, sessionToday, sessionUpcoming]),
        ),
      ),
    );
    renderSessionsPage();

    expect(await screen.findByText("HÔM NAY")).toBeInTheDocument();
    const centerCard = screen.getByRole("link", {
      name: new RegExp(formatSessionDate(sessionToday.session_date)),
    });
    // Today's pending session reads as overdue-style "chưa điểm danh".
    expect(centerCard).toHaveTextContent("Chưa điểm danh");
    expect(screen.getByRole("button", { name: "Buổi kế tiếp" })).toBeEnabled();
  });

  it("steps the anchor back one session with the previous arrow", async () => {
    const { router } = renderSessionsPageWithAttendanceStub();

    await screen.findByText("ĐANG XEM");
    await userEvent.click(screen.getByRole("button", { name: "Buổi trước" }));

    expect(router.state.location.pathname).toBe(`/sessions/${sessionConfirmed.id}/attendance`);
  });

  it("navigates to the picked day's session from the month calendar", async () => {
    const { router } = renderSessionsPageWithAttendanceStub();

    await screen.findByText("ĐANG XEM");
    await userEvent.click(screen.getByRole("button", { name: "Mở lịch tháng" }));

    const dialog = await screen.findByRole("dialog");
    await userEvent.click(
      await within(dialog).findByRole("button", {
        name: formatSessionDate(sessionUpcoming.session_date),
      }),
    );

    await waitFor(() => {
      expect(router.state.location.pathname).toBe(`/sessions/${sessionUpcoming.id}/attendance`);
    });
  });

  it("lets an explicit tab click override the ?class_id= link", async () => {
    useSixClasses();
    signInAs(testPrimaryTeacher);
    renderWithProviders(<SessionsPage />, {
      route: "/sessions?class_id=90000000-0000-4000-8000-000000000011",
      path: "/sessions",
    });
    await screen.findByRole("tab", { name: "Văn 6A" });

    await userEvent.click(screen.getByRole("tab", { name: "Toán 7B" }));

    expect(screen.getByRole("tab", { name: "Toán 7B" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Văn 6A" })).toHaveAttribute("aria-selected", "false");
  });
});
