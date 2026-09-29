import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { renderWithProviders } from "@/test/utils";

import { BlockingSessionsPanel } from "../components/blocking-sessions-panel";
import type { BlockingSession } from "../schemas/billing-schemas";

function blockingSession(
  overrides: Partial<BlockingSession> & Pick<BlockingSession, "session_id">,
): BlockingSession {
  return {
    class_id: "class-toan",
    class_name: "Toán 6A",
    session_date: "2026-09-01",
    start_time: "18:00",
    status: "planned",
    expected_student_count: 20,
    days_overdue: 1,
    ...overrides,
  };
}

const sessions = [
  blockingSession({ session_id: "toan-1", session_date: "2026-09-08", days_overdue: 20 }),
  blockingSession({ session_id: "toan-2", session_date: "2026-09-15", days_overdue: 13 }),
  blockingSession({
    session_id: "van-1",
    class_id: "class-van",
    class_name: "Văn 7B",
    session_date: "2026-09-02",
    days_overdue: 26,
  }),
];

describe("BlockingSessionsPanel", () => {
  it("renders nothing without blocking sessions", () => {
    renderWithProviders(<BlockingSessionsPanel sessions={[]} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("offers one primary action that opens the most overdue session", () => {
    renderWithProviders(<BlockingSessionsPanel sessions={sessions} />);

    const cta = screen.getByRole("link", { name: "Điểm danh buổi còn thiếu (3)" });
    expect(cta).toHaveAttribute("href", "/sessions/van-1/attendance");
  });

  it("groups the remaining sessions per class behind collapsed disclosures", async () => {
    renderWithProviders(<BlockingSessionsPanel sessions={sessions} />);

    const toan = screen.getByText("Toán 6A — 2 buổi").closest("details");
    const van = screen.getByText("Văn 7B — 1 buổi").closest("details");
    expect(toan).not.toHaveAttribute("open");
    expect(van).not.toHaveAttribute("open");

    await userEvent.click(screen.getByText("Toán 6A — 2 buổi"));

    const links = within(toan as HTMLElement).getAllByRole("link");
    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "/sessions/toan-1/attendance",
      "/sessions/toan-2/attendance",
    ]);
    expect(links[0]).toHaveAccessibleName(expect.stringMatching(/^Điểm danh Toán 6A, /));
  });
});
