import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import {
  enrollmentActive,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
  studentOnlyChild,
  studentSiblingOne,
} from "@/features/roster/__tests__/roster-handlers";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassbookPage } from "../pages/classbook-page";
import {
  getTeachingApiStore,
  resetTeachingApiStore,
  seedScoreComponents,
  seedTeachingSession,
  teachingHandlers,
} from "./teaching-handlers";

/** `classWithSchedule.id` in `roster-handlers.ts` — the only seeded class. */
const CLASS_ID = "70000000-0000-4000-8000-000000000001";
const HELD_SESSION_ID = "session-05";
const LONG_NAME = "Kiểm tra thường xuyên lần 1 học kỳ I năm học này";
const COMPONENT_LONG = { id: "comp-long", name: LONG_NAME, position: 2 };
const COMPONENT_15P = { id: "comp-15p", name: "15 phút", position: 1 };

const studentLate = {
  ...studentOnlyChild,
  id: "50000000-0000-4000-8000-0000000000b1",
  full_name: "Lê Thị Bình",
};
const studentAbsent = {
  ...studentOnlyChild,
  id: "50000000-0000-4000-8000-0000000000b2",
  full_name: "Phạm Quang Huy",
};

function enrollMixedAttendance() {
  const store = getRosterStore();
  for (const [index, student] of [studentLate, studentAbsent].entries()) {
    store.students.push(student);
    store.enrollments.push({
      ...enrollmentActive,
      id: `80000000-0000-4000-8000-0000000000b${index + 1}`,
      student_id: student.id,
      student_name: student.full_name,
    });
  }
  store.attendanceStatus[HELD_SESSION_ID] = {
    [studentLate.id]: "late",
    [studentAbsent.id]: "absent",
  };
}

function renderClassbookPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ClassbookPage />, { route: "/classbook", path: "/classbook" });
}

async function openTable(user: ReturnType<typeof userEvent.setup>) {
  renderClassbookPage();
  await user.click(await screen.findByRole("button", { name: /Th 4, 05\/08/ }));
  await user.click(await screen.findByRole("button", { name: "Mở bảng đầy đủ" }));
  return await screen.findByRole("dialog", { name: "Bảng điểm — buổi Th 4, 05/08" });
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-08-20T10:00:00"));
  resetRosterStore();
  resetTeachingApiStore();
  server.use(...rosterHandlers, ...teachingHandlers);
  for (const session of getRosterStore().sessions) {
    seedTeachingSession({
      id: session.id,
      class_id: session.class_id,
      session_date: session.session_date,
    });
  }
  seedScoreComponents(CLASS_ID, [COMPONENT_LONG, COMPONENT_15P]);
  enrollMixedAttendance();
  localStorage.clear();
});

afterEach(() => {
  useAuthStore.getState().clearSession();
  vi.useRealTimers();
});

describe("ScoreTableModal", () => {
  it("lays out sticky headers, a column per component, TB, and one absent row", async () => {
    const user = userEvent.setup();
    const dialog = await openTable(user);

    const headers = within(dialog).getAllByRole("columnheader");
    expect(headers.map((header) => header.textContent)).toEqual([
      "Học sinh",
      "15 phút",
      LONG_NAME,
      "TB",
    ]);
    expect(headers[0]).toHaveClass("sticky", "left-0", "top-0");
    expect(headers[2]).toHaveAttribute("aria-label", LONG_NAME);
    expect(headers[2]!.querySelector("span")).toHaveClass("line-clamp-2");

    const rowHeaders = within(dialog).getAllByRole("rowheader");
    expect(rowHeaders.map((header) => header.textContent)).toEqual([
      "Nguyễn Văn AnAnh, lớp 9A",
      "Lê Thị BìnhĐi muộn",
    ]);
    expect(rowHeaders[0]).toHaveClass("sticky", "left-0");
    expect(within(dialog).getAllByRole("textbox")).toHaveLength(4);
    expect(within(dialog).getByText("Phạm Quang Huy")).toBeInTheDocument();
    expect(within(dialog).getByText("Vắng (1):")).toBeInTheDocument();
    expect(within(dialog).getByRole("status")).toHaveTextContent(
      "0/2 học sinh đã chấm · 0 ô chưa lưu",
    );
  });

  it("computes the row average in place and walks the column with Enter", async () => {
    const user = userEvent.setup();
    const dialog = await openTable(user);

    const anQuiz = within(dialog).getByLabelText("Điểm 15 phút Nguyễn Văn An");
    await user.click(anQuiz);
    await user.keyboard("{Shift>}{Enter}{/Shift}");
    expect(anQuiz).toHaveFocus();

    await user.keyboard("7");
    await user.tab();
    await user.keyboard("8");
    await user.tab();
    const anRow = within(dialog)
      .getByRole("rowheader", { name: /Nguyễn Văn An/ })
      .closest("tr")!;
    expect(within(anRow).getAllByRole("cell").at(-1)).toHaveTextContent("7,5");

    await user.click(anQuiz);
    await user.keyboard("{Enter}");
    expect(within(dialog).getByLabelText("Điểm 15 phút Lê Thị Bình")).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(within(dialog).getByLabelText("Điểm 15 phút Lê Thị Bình")).toHaveFocus();
  });

  it("shares the draft with the expanded row: closing keeps the unsaved cell visible there", async () => {
    const user = userEvent.setup();
    const dialog = await openTable(user);

    await user.type(within(dialog).getByLabelText("Điểm 15 phút Nguyễn Văn An"), "9");
    await user.click(within(dialog).getByRole("button", { name: "Đóng" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    const panelInput = screen.getByLabelText("Điểm 15 phút Nguyễn Văn An");
    expect(panelInput).toHaveValue("9");
    expect(panelInput).toHaveAttribute("data-state", "dirty");
    expect(screen.getByRole("status")).toHaveTextContent("1 ô chưa lưu");
  });

  it("saves once from the footer and the expanded row shows the cell saved", async () => {
    let putCount = 0;
    server.use(
      http.put(`${API_URL}/sessions/:id/scores`, async ({ request }) => {
        putCount += 1;
        const entries = (await request.json()) as { score: number }[];
        for (const entry of entries) {
          getTeachingApiStore().componentScores.set(
            `${HELD_SESSION_ID}#${studentSiblingOne.id}#${COMPONENT_15P.id}`,
            entry.score,
          );
        }
        return HttpResponse.json(
          ok([{ student_id: studentSiblingOne.id, component_id: COMPONENT_15P.id, score: 9 }]),
        );
      }),
    );
    const user = userEvent.setup();
    const dialog = await openTable(user);

    await user.type(within(dialog).getByLabelText("Điểm 15 phút Nguyễn Văn An"), "9");
    await user.click(within(dialog).getByRole("button", { name: "Lưu điểm" }));
    expect(
      await screen.findByText("Đã lưu điểm thành phần (1 ô) — buổi Th 4, 05/08"),
    ).toBeInTheDocument();
    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(putCount).toBe(1);
    const panelInput = screen.getByLabelText("Điểm 15 phút Nguyễn Văn An");
    expect(panelInput).toHaveValue("9");
    expect(panelInput).toHaveAttribute("data-state", "saved");
  });
});
