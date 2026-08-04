import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { AttendancePage } from "../pages/attendance-page";
import {
  attendanceHandlers,
  resetAttendanceStore,
  sessionCancelled,
  sessionConfirmed,
  sessionInClosedPeriod,
  sessionUnconfirmedPast,
} from "./attendance-handlers";

const capturedRequests: { method: string; url: string }[] = [];
server.events.on("request:start", ({ request }) => {
  capturedRequests.push({ method: request.method, url: request.url });
});

function renderAttendancePage(sessionId: string) {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<AttendancePage />, {
    route: `/sessions/${sessionId}/attendance`,
    path: "/sessions/:id/attendance",
  });
}

beforeEach(() => {
  resetAttendanceStore();
  server.use(...attendanceHandlers);
  capturedRequests.length = 0;
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("AttendancePage", () => {
  it("confirms a 30-student session with two absentees in three interactions total", async () => {
    const user = userEvent.setup();
    renderAttendancePage(sessionUnconfirmedPast.id);

    // The only POST so far is the closed-period pre-check, not a per-row write.
    const attendanceWrites = () =>
      capturedRequests.filter(
        (r) => r.method === "POST" && /\/sessions\/.+\/attendance$/.test(r.url),
      );

    // Everyone renders present by default — no per-row network call yet.
    const rows = await screen.findAllByRole("button", { name: /Học sinh|Nguyễn Văn An/ });
    expect(rows).toHaveLength(30);
    for (const row of rows) {
      expect(row).toHaveAttribute("aria-pressed", "false");
    }
    expect(attendanceWrites()).toHaveLength(0);

    // Interaction 1 and 2: tap the two absentees.
    const firstAbsentRow = screen.getByRole("button", { name: /Học sinh 1$/ });
    const secondAbsentRow = screen.getByRole("button", { name: /Học sinh 2$/ });
    await user.click(firstAbsentRow);
    await user.click(secondAbsentRow);
    expect(firstAbsentRow).toHaveAttribute("aria-pressed", "true");
    expect(secondAbsentRow).toHaveAttribute("aria-pressed", "true");
    // Still purely local state — no network yet from the two taps.
    expect(attendanceWrites()).toHaveLength(0);

    // Interaction 3: the single confirm tap.
    const confirmButton = screen.getByRole("button", { name: /vắng/ });
    expect(confirmButton).toHaveTextContent("2 vắng");
    await user.click(confirmButton);

    // Exactly three user interactions (two taps, one confirm) produced exactly one write.
    await waitFor(() => {
      expect(attendanceWrites()).toHaveLength(1);
    });
  });

  it("pre-marks the previous absentees when reopening a confirmed session", async () => {
    renderAttendancePage(sessionConfirmed.id);

    const previouslyAbsentRow = await screen.findByRole("button", { name: /Học sinh 1$/ });
    const previouslyPresentRow = await screen.findByRole("button", { name: /Học sinh 3$/ });
    await waitFor(() => {
      expect(previouslyAbsentRow).toHaveAttribute("aria-pressed", "true");
    });
    expect(previouslyPresentRow).toHaveAttribute("aria-pressed", "false");
  });

  it("shows the closed-period warning and the adjustment button copy", async () => {
    renderAttendancePage(sessionInClosedPeriod.id);

    expect(await screen.findByRole("alert")).toHaveTextContent("đã chốt sổ");
    const confirmButton = await screen.findByRole("button", { name: /vắng/ });
    expect(confirmButton).toHaveTextContent("LƯU VÀ TẠO ĐIỀU CHỈNH");
  });

  it("renders a badge, not a muted suffix, for same-name siblings", async () => {
    renderAttendancePage(sessionUnconfirmedPast.id);

    const [firstSiblingRow, secondSiblingRow] = await screen.findAllByRole("button", {
      name: /Nguyễn Văn An/,
    });
    // A duplicate name promotes `display_note` to a badge (`rounded-full`
    // pill), not the plain muted suffix span single-child students get.
    expect(within(firstSiblingRow!).getByText("Anh, lớp 9A")).toHaveClass("rounded-full");
    expect(within(secondSiblingRow!).getByText("Em, lớp 7B")).toHaveClass("rounded-full");
  });

  it("shows the cancelled-session empty state and bills nobody", async () => {
    renderAttendancePage(sessionCancelled.id);

    expect(await screen.findByText("Buổi học đã huỷ")).toBeInTheDocument();
    expect(screen.getByText(sessionCancelled.cancel_reason!)).toBeInTheDocument();
    expect(screen.getByText(/Không tính tiền cho học sinh nào/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /vắng/ })).not.toBeInTheDocument();
  });
});
