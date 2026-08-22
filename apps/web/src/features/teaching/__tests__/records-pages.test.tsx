import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import {
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
  studentSiblingOne,
} from "@/features/roster/__tests__/roster-handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { RecordsPage } from "../pages/records-page";
import { StudentRecordPage } from "../pages/student-record-page";
import {
  getTeachingApiStore,
  resetTeachingApiStore,
  seedMark,
  seedTeachingSession,
  teachingHandlers,
} from "./teaching-handlers";

const AN = studentSiblingOne.id;

function seedScores(scores: Record<string, Record<string, number>>) {
  for (const [sessionId, byStudent] of Object.entries(scores)) {
    for (const [studentId, score] of Object.entries(byStudent)) {
      seedMark(sessionId, studentId, { score });
    }
  }
}

function renderRecordsPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<RecordsPage />, {
    route: "/records",
    path: "/records",
    extraRoutes: [{ path: "/records/:studentId", element: <StudentRecordPage /> }],
  });
}

function renderStudentRecordPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<StudentRecordPage />, {
    route: `/records/${AN}`,
    path: "/records/:studentId",
  });
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-08-20T10:00:00"));
  resetRosterStore();
  resetTeachingApiStore();
  server.use(...rosterHandlers, ...teachingHandlers);
  // The month-marks read only attributes rows to sessions it knows about.
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

describe("RecordsPage", () => {
  it("lists per-student average, trend, absences, and a dash for NGÀY SINH", async () => {
    seedScores({ "session-05": { [AN]: 8.5 }, "session-12": { [AN]: 9 } });
    renderRecordsPage();

    const row = (await screen.findByText("Nguyễn Văn An")).parentElement!;
    // No dob data exists anywhere — the column is always a dash.
    expect(within(row).getByText("—")).toBeInTheDocument();
    expect(await within(row).findByText("8.8")).toBeInTheDocument();
    // Two scores are below the 4-score floor for a verdict.
    expect(within(row).getByText("Chưa đủ dữ liệu")).toBeInTheDocument();
    expect(within(row).getByText("0")).toBeInTheDocument();
  });

  it("counts absences and drops the absent session from the average", async () => {
    getRosterStore().absences["session-12"] = [AN];
    seedScores({ "session-05": { [AN]: 7 } });
    renderRecordsPage();

    const row = (await screen.findByText("Nguyễn Văn An")).parentElement!;
    expect(await within(row).findByText("1 buổi")).toBeInTheDocument();
    expect(within(row).getByText("7.0")).toBeInTheDocument();
  });

  it("exports the list CSV and navigates to the student detail", async () => {
    const createObjectURL = vi.fn<(blob: Blob) => string>(() => "blob:records");
    Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() });
    seedScores({ "session-05": { [AN]: 8.5 }, "session-12": { [AN]: 9 } });
    const user = userEvent.setup();
    renderRecordsPage();
    await screen.findByText("Nguyễn Văn An");
    await screen.findByText("8.8");

    await user.click(screen.getByRole("button", { name: /Tải danh sách \(CSV\)/ }));
    expect(await screen.findByText("Đã tải HocSinh_Toán_6A.csv")).toBeInTheDocument();
    const text = await createObjectURL.mock.calls[0]![0].text();
    expect(text).toContain('"Họ tên";"Ngày sinh";"Lớp";"Nhập học";"Điểm TB"');
    expect(text).toContain('"Nguyễn Văn An";"—";"Toán 6A";"2026-01-05";"8.8"');

    await user.click(screen.getByRole("button", { name: "Xem hồ sơ" }));
    expect(await screen.findByRole("heading", { name: "Nguyễn Văn An" })).toBeInTheDocument();
    expect(screen.getByText("← Hồ sơ học sinh")).toBeInTheDocument();
  });
});

describe("StudentRecordPage", () => {
  it("derives stat cards, score bars and lesson labels from the student's month", async () => {
    seedScores({ "session-05": { [AN]: 8.5 }, "session-12": { [AN]: 5.5 } });
    renderStudentRecordPage();

    expect(await screen.findByRole("heading", { name: "Nguyễn Văn An" })).toBeInTheDocument();
    expect(screen.getByText("Toán 6A · Nhập học 05/01/2026")).toBeInTheDocument();

    const averageCard = (await screen.findByText("ĐIỂM TB THÁNG 8")).parentElement!;
    expect(await within(averageCard).findByText("7.0")).toBeInTheDocument();
    expect(within(averageCard).getByText("2 bài kiểm tra buổi")).toBeInTheDocument();

    const trendCard = screen.getByText("XU HƯỚNG").parentElement!;
    expect(within(trendCard).getByText("→ Chưa đủ dữ liệu")).toBeInTheDocument();

    const attendanceCard = screen.getByText("CHUYÊN CẦN").parentElement!;
    expect(within(attendanceCard).getByText("100%")).toBeInTheDocument();
    expect(within(attendanceCard).getByText("0 buổi vắng")).toBeInTheDocument();

    const chart = screen.getByText(/ĐIỂM KIỂM TRA TỪNG BUỔI — THÁNG 08/).parentElement!;
    expect(within(chart).getByText("8.5")).toBeInTheDocument();
    expect(within(chart).getByText("5.5")).toBeInTheDocument();
    expect(within(chart).getByText("05")).toBeInTheDocument();
    expect(within(chart).getByText("12")).toBeInTheDocument();

    // Lesson axis skips the cancelled day-8 session: day 12 is Bài 2.
    expect(screen.getByText("Bài 1")).toBeInTheDocument();
    expect(screen.getByText("Bài 2")).toBeInTheDocument();
  });

  it("marks absent sessions in chart and table", async () => {
    getRosterStore().absences["session-12"] = [AN];
    renderStudentRecordPage();

    expect(await screen.findByText("Vắng")).toBeInTheDocument();
    const chart = screen.getByText(/ĐIỂM KIỂM TRA TỪNG BUỔI/).parentElement!;
    expect(within(chart).getByText("V")).toBeInTheDocument();
    const attendanceCard = screen.getByText("CHUYÊN CẦN").parentElement!;
    expect(within(attendanceCard).getByText("50%")).toBeInTheDocument();
    expect(within(attendanceCard).getByText("1 buổi vắng")).toBeInTheDocument();
  });

  it("saves an inline personal note on blur and persists it through the API", async () => {
    const user = userEvent.setup();
    renderStudentRecordPage();

    const input = await screen.findByLabelText("Nhận xét buổi Th 4, 05/08");
    await user.type(input, "Hăng hái phát biểu");
    await user.tab();

    expect(await screen.findByText("Đã lưu nhận xét cho Nguyễn Văn An")).toBeInTheDocument();
    expect(getTeachingApiStore().marks.get(`session-05#${AN}`)?.personal_note).toBe(
      "Hăng hái phát biểu",
    );

    // Blurring again without edits must not re-toast or rewrite.
    await user.click(input);
    await user.tab();
    expect(screen.getAllByText("Đã lưu nhận xét cho Nguyễn Văn An")).toHaveLength(1);
  });

  it("exports the student CSV with the prototype's header", async () => {
    const createObjectURL = vi.fn<(blob: Blob) => string>(() => "blob:student");
    Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() });
    seedScores({ "session-05": { [AN]: 8.5 } });
    seedMark("session-05", AN, { score: 8.5, personal_note: "Cần luyện thêm" });
    const user = userEvent.setup();
    renderStudentRecordPage();
    await screen.findByLabelText("Nhận xét buổi Th 4, 05/08");
    await screen.findByDisplayValue("Cần luyện thêm");

    await user.click(screen.getByRole("button", { name: "Tải hồ sơ (CSV)" }));

    expect(await screen.findByText("Đã tải Nguyễn_Văn_An_ky08.csv")).toBeInTheDocument();
    const text = await createObjectURL.mock.calls[0]![0].text();
    expect(text.startsWith('"Buổi";"Bài học";"Trạng thái";"Điểm";"Nhận xét"')).toBe(true);
    expect(text).toContain('"Th 4, 05/08";"Bài 1";"Có mặt";"8.5";"Cần luyện thêm"');
    expect(text).toContain('"Bài 2";"Có mặt";""');
  });
});
