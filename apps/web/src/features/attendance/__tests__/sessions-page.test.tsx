import { screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { formatSessionDate } from "@/lib/utils";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { SessionsPage } from "../pages/sessions-page";
import {
  attendanceHandlers,
  resetAttendanceStore,
  sessionUnconfirmedPast,
} from "./attendance-handlers";

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
});
