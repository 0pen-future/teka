import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import {
  classWithSchedule,
  resetRosterStore,
  rosterHandlers,
} from "@/features/roster/__tests__/roster-handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassbookPage } from "../pages/classbook-page";
import {
  countPendingPlans,
  getTeachingSnapshot,
  lessonPlanKey,
  resetTeachingStoreForTests,
  updateTeachingState,
} from "../lib/teaching-store";

const CENTER = "Trung Tâm Bình Minh";
const CLASS_ID = classWithSchedule.id;
const LESSONS = [
  "Số tự nhiên",
  "Phân số",
  "Số thập phân",
  "Tỉ số phần trăm",
  "Hình học phẳng",
  "Ôn tập cuối kỳ",
];

function seedCurriculum(lessons: string[] = LESSONS) {
  updateTeachingState(CENTER, (state) => ({
    ...state,
    curricula: { ...state.curricula, [CLASS_ID]: { lessons, currentIndex: 0 } },
  }));
}

async function renderCourseView() {
  signInAs(testPrimaryTeacher);
  const user = userEvent.setup();
  renderWithProviders(<ClassbookPage />, { route: "/classbook", path: "/classbook" });
  // Wait for sessions to load: doneCount (2 held) drives every progress number.
  await screen.findByRole("button", { name: /Th 4, 05\/08/ });
  await user.click(screen.getByRole("tab", { name: "Chương trình & giáo án" }));
  return user;
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-08-20T10:00:00"));
  resetRosterStore();
  server.use(...rosterHandlers);
  localStorage.clear();
  resetTeachingStoreForTests();
});

afterEach(() => {
  useAuthStore.getState().clearSession();
  vi.useRealTimers();
});

describe("ClassbookPage course view — curriculum", () => {
  it("creates a curriculum through the editor and shows held-session progress", async () => {
    const user = await renderCourseView();

    expect(screen.getByText(/Chưa có chương trình cho lớp Toán 6A/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "✎ Sửa chương trình" }));
    expect(await screen.findByText("Chương trình — Toán 6A")).toBeInTheDocument();

    // Only 3 named lessons → below the 4-lesson floor, the editor stays open.
    await user.type(screen.getByLabelText("Bài 1"), "Số tự nhiên");
    await user.type(screen.getByLabelText("Bài 2"), "Phân số");
    await user.type(screen.getByLabelText("Bài 3"), "Số thập phân");
    await user.click(screen.getByRole("button", { name: "Lưu chương trình" }));
    expect(await screen.findByText("Chương trình cần ít nhất 4 buổi")).toBeInTheDocument();
    expect(screen.getByText("Chương trình — Toán 6A")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Bài 4"), "Tỉ số phần trăm");
    await user.click(screen.getByRole("button", { name: "Lưu chương trình" }));

    expect(
      await screen.findByText("Đã lưu chương trình Toán 6A — khóa 4 buổi"),
    ).toBeInTheDocument();
    // 2 held sessions this month → lesson 2 is current, lesson 3 is next.
    expect(screen.getByText("Buổi 2/4")).toBeInTheDocument();
    expect(screen.getByText("Bài 2 · Phân số")).toBeInTheDocument();
    expect(screen.getByText("Buổi tới: Bài 3 · Số thập phân")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Xem toàn bộ chương trình" }));
    expect(screen.getByText("Số tự nhiên")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Thu gọn chương trình" })).toBeInTheDocument();
  });
});

describe("ClassbookPage course view — giáo án", () => {
  it("saves a draft, submits it for review, and flips the planned row's chip", async () => {
    seedCurriculum();
    const user = await renderCourseView();

    // nextIndex = done (2) → Bài 3 — the same lesson index the sessions table
    // assigns to the upcoming planned session.
    expect(await screen.findByText("Bài 3/6 · Số thập phân")).toBeInTheDocument();
    expect(screen.getByText("Chưa nộp")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "✎ Soạn giáo án trực tiếp" }));
    expect(await screen.findByText("Soạn giáo án — Bài 3 · Số thập phân")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Mục tiêu buổi học"), "Đọc và viết số thập phân");
    await user.type(screen.getByLabelText(/Hoạt động trên lớp/), "Khởi động 10'\nLuyện tập nhóm");
    await user.type(screen.getByLabelText("Bài tập về nhà"), "Phiếu bài tập số 3");
    await user.click(screen.getByRole("button", { name: "Lưu giáo án" }));

    expect(
      await screen.findByText("Đã lưu giáo án Toán 6A — nộp duyệt khi sẵn sàng"),
    ).toBeInTheDocument();
    expect(screen.getByText("Bản nháp")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Nộp duyệt giáo án" }));
    expect(
      await screen.findByText("Đã nộp giáo án Toán 6A — chờ chủ trung tâm duyệt"),
    ).toBeInTheDocument();
    expect(screen.getByText("Chờ duyệt")).toBeInTheDocument();
    expect(countPendingPlans(getTeachingSnapshot(CENTER))).toBe(1);

    // The same pending status shows on the upcoming session row without reload.
    await user.click(screen.getByRole("tab", { name: "Buổi học & nhận xét" }));
    const plannedRow = await screen.findByRole("button", { name: /Th 4, 19\/08/ });
    expect(within(plannedRow).getByText("Chờ duyệt")).toBeInTheDocument();
  });

  it("shows the owner's redo note and keeps the submit path open", async () => {
    seedCurriculum();
    updateTeachingState(CENTER, (state) => ({
      ...state,
      lessonPlans: {
        [lessonPlanKey(CLASS_ID, 2)]: {
          goal: "Đọc và viết số thập phân",
          activities: ["Khởi động"],
          homework: "Phiếu số 3",
          status: "redo",
          redoNote: "Thiếu phần luyện tập",
        },
      },
    }));
    await renderCourseView();

    expect(await screen.findByText("Cần sửa lại")).toBeInTheDocument();
    expect(screen.getByText("Thiếu phần luyện tập")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Nộp duyệt giáo án" })).toBeInTheDocument();
  });

  it("locks editing while the plan is under or after review", async () => {
    seedCurriculum();
    updateTeachingState(CENTER, (state) => ({
      ...state,
      lessonPlans: {
        [lessonPlanKey(CLASS_ID, 2)]: {
          goal: "Đọc và viết số thập phân",
          activities: ["Khởi động"],
          homework: "Phiếu số 3",
          status: "pending",
          submittedBy: "Cô Lan",
        },
      },
    }));
    await renderCourseView();

    // No save transition exists from pending: both edit paths disappear.
    expect(await screen.findByText("Chờ duyệt")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "✎ Soạn giáo án trực tiếp" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByLabelText("hoặc đính kèm file Word/PDF")).not.toBeInTheDocument();
    expect(
      screen.getByText("Đã nộp duyệt — chờ chủ trung tâm phản hồi trước khi sửa."),
    ).toBeInTheDocument();

    act(() => {
      updateTeachingState(CENTER, (state) => ({
        ...state,
        lessonPlans: {
          [lessonPlanKey(CLASS_ID, 2)]: {
            ...state.lessonPlans[lessonPlanKey(CLASS_ID, 2)]!,
            status: "approved",
          },
        },
      }));
    });
    expect(
      await screen.findByText("Đã duyệt — cần sửa thì nhờ chủ trung tâm mở lại để duyệt lại."),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "✎ Soạn giáo án trực tiếp" }),
    ).not.toBeInTheDocument();
  });

  it("renders the monthly headcount chart and attaches a plan file by name", async () => {
    seedCurriculum();
    const user = await renderCourseView();

    const chart = screen.getByText("SĨ SỐ THEO THÁNG").parentElement!;
    // One enrollment since January → a count of 1 in each of the 5 months.
    expect(within(chart).getAllByText("1")).toHaveLength(5);
    for (const label of ["T4", "T5", "T6", "T7", "T8"]) {
      expect(within(chart).getByText(label)).toBeInTheDocument();
    }
    expect(
      within(chart).getByText("Tái tục T7→T8: 100% — 1/1 học sinh học tiếp"),
    ).toBeInTheDocument();

    await user.upload(
      screen.getByLabelText("hoặc đính kèm file Word/PDF"),
      new File(["x"], "giao-an-tuan-3.docx"),
    );
    expect(await screen.findByText("📎 giao-an-tuan-3.docx")).toBeInTheDocument();
    // Attaching creates a draft, so the submit path opens up.
    expect(screen.getByText("Bản nháp")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Nộp duyệt giáo án" })).toBeInTheDocument();
  });
});
