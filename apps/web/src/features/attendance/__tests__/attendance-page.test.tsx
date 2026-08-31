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

interface CapturedRequest {
  method: string;
  url: string;
  body?: unknown;
}

const capturedRequests: CapturedRequest[] = [];
server.events.on("request:start", ({ request }) => {
  const entry: CapturedRequest = { method: request.method, url: request.url };
  capturedRequests.push(entry);
  if (request.method === "POST") {
    void request
      .clone()
      .json()
      .then((body) => {
        entry.body = body;
      })
      .catch(() => undefined);
  }
});

const attendanceWrites = () =>
  capturedRequests.filter((r) => r.method === "POST" && /\/sessions\/.+\/attendance$/.test(r.url));

function renderAttendancePage(sessionId: string) {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<AttendancePage />, {
    route: `/sessions/${sessionId}/attendance`,
    path: "/sessions/:id/attendance",
  });
}

function studentRow(name: string | RegExp) {
  return screen.getByRole("radiogroup", { name });
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
  it("defaults the whole class to Đúng giờ and confirms in a single tap with no marks", async () => {
    const user = userEvent.setup();
    renderAttendancePage(sessionUnconfirmedPast.id);

    // Every student renders as a radiogroup with Đúng giờ pre-checked —
    // purely local state, no per-row network call.
    const rows = await screen.findAllByRole("radiogroup");
    expect(rows).toHaveLength(30);
    expect(within(rows[0]!).getByRole("radio", { name: "Đúng giờ" })).toBeChecked();
    expect(within(rows[0]!).getByRole("radio", { name: "Muộn" })).not.toBeChecked();
    expect(attendanceWrites()).toHaveLength(0);

    // The count chip reflects the all-on-time default; zero-count chips
    // (Muộn/Vắng/Có lý do) stay hidden.
    expect(screen.getByText("Đúng giờ 30")).toBeInTheDocument();
    expect(screen.queryByText(/^Vắng \d/)).not.toBeInTheDocument();

    // One tap: the confirm bar carries no exception suffix. (findBy: the
    // label stays "Đang tải…" until the class's role check resolves.)
    const confirmButton = await screen.findByRole("button", { name: /^XÁC NHẬN$/ });
    await user.click(confirmButton);

    await waitFor(() => {
      expect(attendanceWrites()).toHaveLength(1);
    });
    expect(attendanceWrites()[0]!.body).toMatchObject({ marks: [] });
  });

  it("marks late, absent, and excused locally and sends only the exceptions", async () => {
    const user = userEvent.setup();
    renderAttendancePage(sessionUnconfirmedPast.id);

    await screen.findAllByRole("radiogroup");
    await user.click(within(studentRow(/Học sinh 1$/)).getByRole("radio", { name: "Vắng" }));
    await user.click(within(studentRow(/Học sinh 2$/)).getByRole("radio", { name: "Muộn" }));
    await user.click(within(studentRow(/Học sinh 3$/)).getByRole("radio", { name: "Có lý do" }));

    expect(within(studentRow(/Học sinh 1$/)).getByRole("radio", { name: "Vắng" })).toBeChecked();
    expect(within(studentRow(/Học sinh 2$/)).getByRole("radio", { name: "Muộn" })).toBeChecked();
    expect(
      within(studentRow(/Học sinh 3$/)).getByRole("radio", { name: "Có lý do" }),
    ).toBeChecked();

    // Picking Có lý do opens the quick note input; the note becomes the
    // "Vắng có phép" subtitle under the student's name.
    const noteInput = screen.getByRole("textbox", { name: "Lý do của Học sinh 3" });
    await user.type(noteInput, "mẹ báo ốm");
    expect(screen.getByText(/Vắng có phép — mẹ báo ốm/)).toBeInTheDocument();

    // Chips show the live split, hiding nothing that is non-zero.
    expect(screen.getByText("Đúng giờ 27")).toBeInTheDocument();
    expect(screen.getByText("Muộn 1")).toBeInTheDocument();
    expect(screen.getByText("Vắng 1")).toBeInTheDocument();
    expect(screen.getByText("Có lý do 1")).toBeInTheDocument();

    // Still zero writes: the whole sheet is local until the single confirm.
    expect(attendanceWrites()).toHaveLength(0);

    const confirmButton = screen.getByRole("button", { name: /XÁC NHẬN · 1 VẮNG · 1 MUỘN/ });
    await user.click(confirmButton);

    await waitFor(() => {
      expect(attendanceWrites()).toHaveLength(1);
    });
    const body = attendanceWrites()[0]!.body as { marks: unknown[] };
    expect(body.marks).toHaveLength(3);
    expect(body.marks).toEqual(
      expect.arrayContaining([
        { student_id: "student-001", status: "absent" },
        { student_id: "student-002", status: "late" },
        { student_id: "student-003", status: "excused", note: "mẹ báo ốm" },
      ]),
    );
  });

  it("returns a student to Đúng giờ when their selected status is tapped again", async () => {
    const user = userEvent.setup();
    renderAttendancePage(sessionUnconfirmedPast.id);

    await screen.findAllByRole("radiogroup");
    const absentRadio = within(studentRow(/Học sinh 1$/)).getByRole("radio", { name: "Vắng" });
    await user.click(absentRadio);
    expect(absentRadio).toBeChecked();

    await user.click(absentRadio);
    expect(absentRadio).not.toBeChecked();
    expect(
      within(studentRow(/Học sinh 1$/)).getByRole("radio", { name: "Đúng giờ" }),
    ).toBeChecked();
  });

  it("reloads a confirmed session's four statuses into the sheet", async () => {
    renderAttendancePage(sessionConfirmed.id);

    await screen.findAllByRole("radiogroup");
    await waitFor(() => {
      expect(within(studentRow(/Học sinh 1$/)).getByRole("radio", { name: "Vắng" })).toBeChecked();
    });
    expect(within(studentRow(/Học sinh 2$/)).getByRole("radio", { name: "Muộn" })).toBeChecked();
    expect(
      within(studentRow(/Học sinh 3$/)).getByRole("radio", { name: "Có lý do" }),
    ).toBeChecked();
    expect(
      within(studentRow(/Học sinh 4$/)).getByRole("radio", { name: "Đúng giờ" }),
    ).toBeChecked();

    // The stored excused note comes back as the subtitle.
    expect(screen.getByText(/Vắng có phép — mẹ báo ốm/)).toBeInTheDocument();
  });

  it("shows the confirmed state on the button and only saves again after an edit", async () => {
    const user = userEvent.setup();
    renderAttendancePage(sessionConfirmed.id);

    // No local edits yet — the button reports state; a tap only explains
    // itself in a toast, it writes nothing.
    const stateButton = await screen.findByRole("button", { name: /ĐÃ XÁC NHẬN/ });
    await user.click(stateButton);
    expect(await screen.findByText("Buổi này đã xác nhận rồi")).toBeInTheDocument();
    expect(attendanceWrites()).toHaveLength(0);

    // Marking one more student absent reopens the save with the live split:
    // the pre-existing absentee plus the fresh one, and the pre-existing late.
    await user.click(within(studentRow(/Học sinh 4$/)).getByRole("radio", { name: "Vắng" }));
    const saveButton = screen.getByRole("button", { name: /XÁC NHẬN · 2 VẮNG · 1 MUỘN/ });
    await user.click(saveButton);
    await waitFor(() => {
      expect(attendanceWrites()).toHaveLength(1);
    });
    const body = attendanceWrites()[0]!.body as { marks: { student_id: string }[] };
    expect(body.marks).toHaveLength(4);
  });

  it("shows the closed-period warning and the adjustment button copy", async () => {
    renderAttendancePage(sessionInClosedPeriod.id);

    expect(await screen.findByRole("alert")).toHaveTextContent("đã chốt sổ");
    expect(
      await screen.findByRole("button", { name: /LƯU VÀ TẠO ĐIỀU CHỈNH/ }),
    ).toBeInTheDocument();
  });

  it("renders a badge, not a muted suffix, for same-name siblings", async () => {
    renderAttendancePage(sessionUnconfirmedPast.id);

    const [firstSiblingRow, secondSiblingRow] = await screen.findAllByRole("radiogroup", {
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
    expect(screen.queryByRole("button", { name: /XÁC NHẬN/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
  });
});
