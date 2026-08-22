import { screen } from "@testing-library/react";
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
import {
  attendanceHandlers,
  fixtureClass,
  resetAttendanceStore,
  sessionUnconfirmedPast,
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
