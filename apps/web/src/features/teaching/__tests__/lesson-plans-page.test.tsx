import { screen, within } from "@testing-library/react";
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
import { LessonPlansPage } from "../pages/lesson-plans-page";
import { lessonPlanKey } from "../lib/teaching-store";
import {
  getTeachingApiStore,
  resetTeachingApiStore,
  seedCurriculum,
  seedPlan,
  teachingHandlers,
} from "./teaching-handlers";
import type { PlanResponse } from "../schemas/teaching-schemas";

const CLASS_ID = classWithSchedule.id;
const LESSONS = ["Số tự nhiên", "Phân số", "Số thập phân", "Hình học", "Tỉ số", "Ôn tập"];
// Two held sessions in the frozen August window → the upcoming lesson is
// index 2, the same key the teacher's next-plan card writes.
const PLAN_KEY = lessonPlanKey(CLASS_ID, 2);

function seedNextPlan(overrides: Partial<PlanResponse> & Pick<PlanResponse, "status">) {
  seedPlan(CLASS_ID, 2, {
    goal: "Hiểu số thập phân",
    activities: ["Khởi động 10'", "Luyện tập 30'"],
    homework: "Phiếu 12",
    submitted_by_name: "Cô Lan",
    ...overrides,
  });
}

function renderPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<LessonPlansPage />, {
    route: "/lesson-plans",
    path: "/lesson-plans",
  });
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-08-20T10:00:00"));
  resetRosterStore();
  resetTeachingApiStore();
  server.use(...rosterHandlers, ...teachingHandlers);
  localStorage.clear();
});

afterEach(() => {
  useAuthStore.getState().clearSession();
  vi.useRealTimers();
});

describe("LessonPlansPage queue", () => {
  it("lists the class with its next lesson, teacher and pending chip", async () => {
    seedCurriculum(CLASS_ID, LESSONS);
    seedNextPlan({ status: "pending" });
    renderPage();

    expect(
      await screen.findByText(
        "Còn 1 giáo án buổi tới chờ duyệt — duyệt xong giáo viên mới lên lớp.",
      ),
    ).toBeInTheDocument();
    const row = await screen.findByRole("button", { name: /Bài 3\/6 · Số thập phân/ });
    expect(within(row).getByText("Toán 6A")).toBeInTheDocument();
    expect(within(row).getByText("Cô Lan")).toBeInTheDocument();
    expect(within(row).getByText("Chờ duyệt")).toBeInTheDocument();

    // Detail panel mirrors the submitted plan.
    expect(screen.getByRole("heading", { name: "Toán 6A — Cô Lan" })).toBeInTheDocument();
    expect(screen.getByText("Hiểu số thập phân")).toBeInTheDocument();
    expect(screen.getByText("Luyện tập 30'")).toBeInTheDocument();
    expect(screen.getByText("Phiếu 12")).toBeInTheDocument();
  });
});

describe("LessonPlansPage review actions", () => {
  it("approves with a comment, then reopens back to pending", async () => {
    seedCurriculum(CLASS_ID, LESSONS);
    seedNextPlan({ status: "pending" });
    const user = userEvent.setup();
    renderPage();

    const textarea = await screen.findByLabelText("NHẬN XÉT CỦA CHỦ TRUNG TÂM");
    await user.type(textarea, "Tốt lắm");
    await user.click(screen.getByRole("button", { name: "Duyệt giáo án" }));

    expect(
      await screen.findByText("Đã duyệt giáo án Toán 6A — giáo viên thấy trạng thái trong sổ lớp"),
    ).toBeInTheDocument();
    expect(await screen.findByText("Không có giáo án nào chờ duyệt.")).toBeInTheDocument();
    expect(screen.getAllByText("Đã duyệt").length).toBeGreaterThanOrEqual(1);
    const approved = getTeachingApiStore().plans.get(PLAN_KEY)!;
    expect(approved.status).toBe("approved");
    expect(approved.owner_comment).toBe("Tốt lắm");

    await user.click(screen.getByRole("button", { name: "Mở lại để duyệt lại" }));
    expect(await screen.findByLabelText("NHẬN XÉT CỦA CHỦ TRUNG TÂM")).toBeInTheDocument();
    const reopened = getTeachingApiStore().plans.get(PLAN_KEY)!;
    expect(reopened.status).toBe("pending");
    expect(reopened.owner_comment).toBeNull();
    expect(
      screen.getByText("Còn 1 giáo án buổi tới chờ duyệt — duyệt xong giáo viên mới lên lớp."),
    ).toBeInTheDocument();
  });

  it("blocks Yêu cầu sửa until a comment exists, then stores the redo note", async () => {
    seedCurriculum(CLASS_ID, LESSONS);
    seedNextPlan({ status: "pending" });
    const user = userEvent.setup();
    renderPage();

    const redoButton = await screen.findByRole("button", { name: "Yêu cầu sửa" });
    expect(redoButton).toBeDisabled();
    expect(screen.getByText("Ghi rõ cần sửa gì để giáo viên biết đường sửa.")).toBeInTheDocument();

    await user.type(screen.getByLabelText("NHẬN XÉT CỦA CHỦ TRUNG TÂM"), "Thiếu phần luyện tập");
    expect(redoButton).toBeEnabled();
    await user.click(redoButton);

    expect(
      await screen.findByText("Đã yêu cầu sửa giáo án Toán 6A — ghi chú hiển thị trong sổ lớp"),
    ).toBeInTheDocument();
    // The coral box quotes the note back, and the owner can withdraw it.
    expect(await screen.findByText("Thiếu phần luyện tập")).toBeInTheDocument();
    const plan = getTeachingApiStore().plans.get(PLAN_KEY)!;
    expect(plan.status).toBe("redo");
    expect(plan.redo_note).toBe("Thiếu phần luyện tập");
    expect(screen.getAllByText("Cần sửa lại").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByRole("button", { name: "Mở lại để duyệt lại" })).toBeInTheDocument();
  });

  it("shows the not-submitted state with an honest reminder toast", async () => {
    seedCurriculum(CLASS_ID, LESSONS);
    const user = userEvent.setup();
    renderPage();

    expect(
      await screen.findByText(
        "Chưa có giáo án để duyệt — giáo viên nộp trong màn Quản lý lớp học.",
      ),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Chưa nộp").length).toBeGreaterThanOrEqual(1);

    await user.click(screen.getByRole("button", { name: "Nhắc giáo viên nộp qua Zalo" }));
    expect(
      await screen.findByText("Đã tạo lời nhắc nộp giáo án Toán 6A — chưa gửi Zalo tự động"),
    ).toBeInTheDocument();
  });
});

describe("review loop across pages", () => {
  it("makes the owner's redo note visible on the teacher's classbook", async () => {
    seedCurriculum(CLASS_ID, LESSONS);
    seedNextPlan({ status: "pending" });
    const user = userEvent.setup();
    const ownerView = renderPage();

    await user.type(
      await screen.findByLabelText("NHẬN XÉT CỦA CHỦ TRUNG TÂM"),
      "Thiếu phần luyện tập",
    );
    await user.click(screen.getByRole("button", { name: "Yêu cầu sửa" }));
    await screen.findByText("Đã yêu cầu sửa giáo án Toán 6A — ghi chú hiển thị trong sổ lớp");
    ownerView.unmount();

    // The redo note round-trips through the API: the teacher's classbook
    // fetches it fresh in its own render.
    signInAs(testPrimaryTeacher);
    renderWithProviders(<ClassbookPage />, { route: "/classbook", path: "/classbook" });
    await screen.findByRole("button", { name: /Th 4, 05\/08/ });
    await user.click(screen.getByRole("tab", { name: "Chương trình & giáo án" }));

    expect(await screen.findByText(/Chủ trung tâm yêu cầu sửa:/)).toBeInTheDocument();
    expect(screen.getByText(/Thiếu phần luyện tập/)).toBeInTheDocument();
    // The teacher can act on it right away: the plan is still submittable.
    expect(screen.getByRole("button", { name: "Nộp duyệt giáo án" })).toBeInTheDocument();
  });
});
