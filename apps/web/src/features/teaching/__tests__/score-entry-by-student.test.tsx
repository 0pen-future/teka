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
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassbookPage } from "../pages/classbook-page";
import {
  getTeachingApiStore,
  resetTeachingApiStore,
  seedComponentScore,
  seedScoreComponents,
  seedTeachingSession,
  teachingHandlers,
} from "./teaching-handlers";

/** `classWithSchedule.id` in `roster-handlers.ts` — the only seeded class. */
const CLASS_ID = "70000000-0000-4000-8000-000000000001";
const HELD_SESSION_ID = "session-05";
// Deliberately out of position order to prove the panel re-sorts columns.
const COMPONENT_MIENG = { id: "comp-mieng", name: "Miệng", position: 2 };
const COMPONENT_15P = { id: "comp-15p", name: "15 phút", position: 1 };

// Extra roster rows staged per test so one held session carries every
// attendance status the panel distinguishes.
const studentLate = {
  ...studentOnlyChild,
  id: "50000000-0000-4000-8000-0000000000a1",
  full_name: "Lê Thị Bình",
};
const studentAbsent = {
  ...studentOnlyChild,
  id: "50000000-0000-4000-8000-0000000000a2",
  full_name: "Phạm Quang Huy",
};
const studentExcused = {
  ...studentOnlyChild,
  id: "50000000-0000-4000-8000-0000000000a3",
  full_name: "Đỗ Thu Hà",
};

function enrollMixedAttendance() {
  const store = getRosterStore();
  for (const [index, student] of [studentLate, studentAbsent, studentExcused].entries()) {
    store.students.push(student);
    store.enrollments.push({
      ...enrollmentActive,
      id: `80000000-0000-4000-8000-0000000000a${index + 1}`,
      student_id: student.id,
      student_name: student.full_name,
    });
  }
  store.attendanceStatus[HELD_SESSION_ID] = {
    [studentLate.id]: "late",
    [studentAbsent.id]: "absent",
    [studentExcused.id]: "excused",
  };
}

function seedTwoComponents() {
  seedScoreComponents(CLASS_ID, [COMPONENT_MIENG, COMPONENT_15P]);
}

function storedScore(studentId: string, componentId: string): number | undefined {
  return getTeachingApiStore().componentScores.get(
    `${HELD_SESSION_ID}#${studentId}#${componentId}`,
  );
}

function renderClassbookPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ClassbookPage />, { route: "/classbook", path: "/classbook" });
}

/** The day-5 held session's table row (its label under the frozen clock). */
async function findHeldRow() {
  return await screen.findByRole("button", { name: /Th 4, 05\/08/ });
}

/** The day-19 planned (not held) session's table row. */
async function findPlannedRow() {
  return await screen.findByRole("button", { name: /Th 4, 19\/08/ });
}

/** Expands a session row; its scores block renders inline without a tab. */
async function openScoresBlock(user: ReturnType<typeof userEvent.setup>, row: HTMLElement) {
  await user.click(row);
  await screen.findByRole("region", { name: /Chi tiết buổi/ });
}

function studentRowButton(name: string) {
  return screen.getByRole("button", { name: new RegExp(name) });
}

beforeEach(() => {
  // Same frozen clock as classbook-page.test.tsx so the fixture sessions and
  // the month window line up. Timers stay real: autosave waits below cover
  // the 800 ms debounce with a generous `waitFor` timeout.
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
  localStorage.clear();
});

afterEach(() => {
  useAuthStore.getState().clearSession();
  vi.useRealTimers();
});

describe("scores block — class without score components", () => {
  it("renders the plain general-score block unchanged and never the by-student entry", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await openScoresBlock(user, await findHeldRow());

    expect(await screen.findByLabelText("Điểm Nguyễn Văn An")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Lưu điểm buổi" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Lưu điểm" })).not.toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "Học sinh" })).not.toBeInTheDocument();
  });
});

describe("scores block — score entry by student", () => {
  it("lists gradable rows with inputs, folds absent rows into a read-only group", async () => {
    seedTwoComponents();
    enrollMixedAttendance();
    seedComponentScore(HELD_SESSION_ID, studentAbsent.id, COMPONENT_MIENG.id, 8);
    const user = userEvent.setup();
    renderClassbookPage();

    await openScoresBlock(user, await findHeldRow());

    expect(
      await screen.findByText(
        "Chấm điểm từng đầu điểm (0–10), tự lưu khi rời ô — điểm thành phần chưa vào báo cáo phụ huynh.",
      ),
    ).toBeInTheDocument();
    const list = screen.getByRole("list", { name: "Học sinh" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(2);
    // First gradable row opens by default, in position order (15 phút first).
    expect(studentRowButton("Nguyễn Văn An")).toHaveAttribute("aria-expanded", "true");
    const openGroup = screen.getByRole("group", { name: "Điểm của Nguyễn Văn An" });
    const inputs = within(openGroup).getAllByRole("textbox");
    expect(inputs.map((input) => input.getAttribute("aria-label"))).toEqual([
      "Điểm 15 phút Nguyễn Văn An",
      "Điểm Miệng Nguyễn Văn An",
    ]);
    expect(inputs[0]).not.toHaveAttribute("type", "number");
    // Late students are gradable; their row opens on demand.
    expect(screen.getByText("Đi muộn")).toBeInTheDocument();
    await user.click(studentRowButton("Lê Thị Bình"));
    expect(screen.getByLabelText("Điểm 15 phút Lê Thị Bình")).toBeInTheDocument();

    // Absent + excused rows live in the collapsed "Vắng" group, text only.
    const absentGroup = screen.getByText("Vắng (2)").closest("details")!;
    expect(within(absentGroup).queryByRole("textbox")).not.toBeInTheDocument();
    expect(within(absentGroup).getByText("Phạm Quang Huy")).toBeInTheDocument();
    expect(within(absentGroup).getByText("Có phép")).toBeInTheDocument();
    expect(within(absentGroup).getByText("Miệng 8")).toBeInTheDocument();

    expect(screen.getByRole("status")).toHaveTextContent("0/2 học sinh đã chấm · 0 ô chưa lưu");
    expect(screen.getByRole("button", { name: "Lưu điểm" })).toBeDisabled();
    const scoresBlock = screen.getByRole("group", { name: "Điểm buổi" });
    expect(within(scoresBlock).queryByRole("table")).not.toBeInTheDocument();
  });

  it("autosaves a committed cell after the debounce with one PUT and one toast", async () => {
    seedTwoComponents();
    const user = userEvent.setup();
    let putCount = 0;
    server.use(
      http.put(`${API_URL}/sessions/:id/scores`, async ({ request }) => {
        putCount += 1;
        const entries = (await request.json()) as unknown[];
        expect(entries).toEqual([
          { student_id: studentSiblingOne.id, component_id: COMPONENT_15P.id, score: 7.5 },
        ]);
        return HttpResponse.json(
          ok([{ student_id: studentSiblingOne.id, component_id: COMPONENT_15P.id, score: 7.5 }]),
        );
      }),
    );
    renderClassbookPage();
    await openScoresBlock(user, await findHeldRow());

    const input = await screen.findByLabelText("Điểm 15 phút Nguyễn Văn An");
    await user.type(input, "7,5");
    await user.tab();

    expect(input).toHaveAttribute("data-state", "dirty");
    expect(screen.getByRole("status")).toHaveTextContent("1/1 học sinh đã chấm · 1 ô chưa lưu");
    expect(screen.getByRole("button", { name: "Lưu điểm" })).toBeEnabled();

    expect(
      await screen.findByText(
        "Đã lưu điểm thành phần (1 ô) — buổi Th 4, 05/08",
        {},
        { timeout: 3000 },
      ),
    ).toBeInTheDocument();
    expect(putCount).toBe(1);
    expect(input).toHaveAttribute("data-state", "saved");
    expect(input).toHaveValue("7.5");
    expect(screen.getByRole("status")).toHaveTextContent("1/1 học sinh đã chấm · 0 ô chưa lưu");
    expect(screen.getByRole("button", { name: "Lưu điểm" })).toBeDisabled();
  });

  it("saves all dirty cells immediately from the footer, sending null for a cleared cell", async () => {
    seedTwoComponents();
    seedComponentScore(HELD_SESSION_ID, studentSiblingOne.id, COMPONENT_MIENG.id, 8);
    const user = userEvent.setup();
    renderClassbookPage();
    await openScoresBlock(user, await findHeldRow());

    const mieng = await screen.findByLabelText("Điểm Miệng Nguyễn Văn An");
    expect(mieng).toHaveValue("8");
    const quiz = screen.getByLabelText("Điểm 15 phút Nguyễn Văn An");
    await user.type(quiz, "9");
    await user.clear(mieng);
    await user.click(screen.getByRole("button", { name: "Lưu điểm" }));

    expect(
      await screen.findByText("Đã lưu điểm thành phần (2 ô) — buổi Th 4, 05/08"),
    ).toBeInTheDocument();
    expect(storedScore(studentSiblingOne.id, COMPONENT_15P.id)).toBe(9);
    expect(storedScore(studentSiblingOne.id, COMPONENT_MIENG.id)).toBeUndefined();
    expect(mieng).toHaveValue("");
    expect(quiz).toHaveValue("9");
    expect(screen.getByRole("button", { name: "Lưu điểm" })).toBeDisabled();
  });

  it("flags an unparsable cell and blocks saving until it is fixed", async () => {
    seedTwoComponents();
    const user = userEvent.setup();
    renderClassbookPage();
    await openScoresBlock(user, await findHeldRow());

    const quiz = await screen.findByLabelText("Điểm 15 phút Nguyễn Văn An");
    await user.type(quiz, "abc");
    await user.tab();

    expect(quiz).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("status")).toHaveTextContent("1 ô không hợp lệ");
    expect(screen.getByRole("button", { name: "Lưu điểm" })).toBeDisabled();

    await user.clear(quiz);
    await user.type(quiz, "6");
    await user.tab();
    expect(quiz).not.toHaveAttribute("aria-invalid");
    await user.click(screen.getByRole("button", { name: "Lưu điểm" }));
    expect(
      await screen.findByText("Đã lưu điểm thành phần (1 ô) — buổi Th 4, 05/08"),
    ).toBeInTheDocument();
    expect(storedScore(studentSiblingOne.id, COMPONENT_15P.id)).toBe(6);
  });

  it("keeps cells dirty and toasts when the save fails", async () => {
    seedTwoComponents();
    const user = userEvent.setup();
    let putCount = 0;
    server.use(
      http.put(`${API_URL}/sessions/:id/scores`, () => {
        putCount += 1;
        return HttpResponse.json(fail("INTERNAL", "boom"), { status: 500 });
      }),
    );
    renderClassbookPage();
    await openScoresBlock(user, await findHeldRow());

    const quiz = await screen.findByLabelText("Điểm 15 phút Nguyễn Văn An");
    await user.type(quiz, "6");
    await user.click(screen.getByRole("button", { name: "Lưu điểm" }));

    expect(
      await screen.findByText("Không lưu được điểm thành phần — vui lòng thử lại"),
    ).toBeInTheDocument();
    expect(putCount).toBe(1);
    expect(quiz).toHaveAttribute("data-state", "dirty");
    expect(quiz).toHaveValue("6");
    expect(screen.getByRole("status")).toHaveTextContent("1 ô chưa lưu");
    expect(screen.getByRole("button", { name: "Lưu điểm" })).toBeEnabled();
  });

  it("moves to the next gradable student on Enter from the last cell", async () => {
    seedTwoComponents();
    enrollMixedAttendance();
    const user = userEvent.setup();
    renderClassbookPage();
    await openScoresBlock(user, await findHeldRow());

    const quiz = await screen.findByLabelText("Điểm 15 phút Nguyễn Văn An");
    await user.click(quiz);
    await user.keyboard("8{Enter}");
    expect(screen.getByLabelText("Điểm Miệng Nguyễn Văn An")).toHaveFocus();
    await user.keyboard("7{Enter}");

    await waitFor(() => {
      expect(screen.getByLabelText("Điểm 15 phút Lê Thị Bình")).toHaveFocus();
    });
    expect(studentRowButton("Lê Thị Bình")).toHaveAttribute("aria-expanded", "true");
    expect(studentRowButton("Nguyễn Văn An")).toHaveAttribute("aria-expanded", "false");
  });

  it("guards closing the row while cells are unsaved and saves on confirm", async () => {
    seedTwoComponents();
    const user = userEvent.setup();
    renderClassbookPage();
    await openScoresBlock(user, await findHeldRow());

    await user.type(await screen.findByLabelText("Điểm 15 phút Nguyễn Văn An"), "9");
    await user.click(screen.getByRole("button", { name: "Đóng chi tiết buổi" }));

    const dialog = await screen.findByRole("dialog", { name: "Còn 1 ô chưa lưu" });
    expect(within(dialog).getByRole("button", { name: "Lưu và đóng" })).toHaveFocus();
    await user.click(within(dialog).getByRole("button", { name: "Lưu và đóng" }));

    expect(
      await screen.findByText("Đã lưu điểm thành phần (1 ô) — buổi Th 4, 05/08"),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Đóng chi tiết buổi" })).not.toBeInTheDocument();
    });
    expect(storedScore(studentSiblingOne.id, COMPONENT_15P.id)).toBe(9);
  });

  it("refuses to save-and-close while an invalid cell remains", async () => {
    seedTwoComponents();
    let putCount = 0;
    server.use(
      http.put(`${API_URL}/sessions/:id/scores`, () => {
        putCount += 1;
        return HttpResponse.json(ok([]));
      }),
    );
    const user = userEvent.setup();
    renderClassbookPage();
    await openScoresBlock(user, await findHeldRow());

    await user.type(await screen.findByLabelText("Điểm 15 phút Nguyễn Văn An"), "abc");
    await user.click(screen.getByRole("button", { name: "Đóng chi tiết buổi" }));

    const dialog = await screen.findByRole("dialog", { name: "Còn 1 ô chưa lưu" });
    expect(within(dialog).getByRole("button", { name: "Lưu và đóng" })).toBeDisabled();
    expect(within(dialog).getByRole("alert")).toHaveTextContent("Còn 1 ô không hợp lệ");
    expect(within(dialog).getByRole("button", { name: "Ở lại" })).toHaveFocus();

    await user.click(within(dialog).getByRole("button", { name: "Ở lại" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByLabelText("Điểm 15 phút Nguyễn Văn An")).toHaveValue("abc");
    expect(putCount).toBe(0);
  });

  it("lets the teacher discard unsaved cells when closing with Escape, without a PUT", async () => {
    seedTwoComponents();
    const user = userEvent.setup();
    let putCount = 0;
    server.use(
      http.put(`${API_URL}/sessions/:id/scores`, () => {
        putCount += 1;
        return HttpResponse.json(ok([]));
      }),
    );
    renderClassbookPage();
    await openScoresBlock(user, await findHeldRow());

    await user.type(await screen.findByLabelText("Điểm 15 phút Nguyễn Văn An"), "9");
    // Escape from the row button routes through the same page-level guard.
    (await findHeldRow()).focus();
    await user.keyboard("{Escape}");

    const dialog = await screen.findByRole("dialog", { name: "Còn 1 ô chưa lưu" });
    await user.click(within(dialog).getByRole("button", { name: "Bỏ thay đổi" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.queryByRole("region", { name: /Chi tiết buổi/ })).not.toBeInTheDocument();
    expect(putCount).toBe(0);
  });

  it("renders a read-only explanation for a session that has not happened yet", async () => {
    seedTwoComponents();
    const user = userEvent.setup();
    renderClassbookPage();
    await openScoresBlock(user, await findPlannedRow());

    expect(await screen.findByText("Chấm điểm sau khi buổi diễn ra.")).toBeInTheDocument();
    const scoresBlock = screen.getByRole("group", { name: "Điểm buổi" });
    expect(within(scoresBlock).queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Lưu điểm" })).not.toBeInTheDocument();
  });
});
