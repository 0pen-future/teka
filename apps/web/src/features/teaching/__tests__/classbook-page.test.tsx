import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL } from "@/test/msw/handlers";
import {
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
} from "@/features/roster/__tests__/roster-handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassbookPage } from "../pages/classbook-page";
import {
  resetTeachingApiStore,
  seedTeachingSession,
  teachingHandlers,
} from "./teaching-handlers";

function renderClassbookPage(route = "/classbook") {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ClassbookPage />, { route, path: "/classbook" });
}

/** The stat card element owning `label` — its value/sub live in siblings. */
function statCard(label: string): HTMLElement {
  return screen.getByText(label).parentElement!;
}

/** The day-5 held session's table row (its label under the frozen clock). */
async function findHeldRow() {
  return await screen.findByRole("button", { name: /Th 4, 05\/08/ });
}

beforeEach(() => {
  // Freeze only Date (timeouts stay real for msw/userEvent): fixtures and
  // the month window both derive from "today".
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-08-20T10:00:00"));
  resetRosterStore();
  resetTeachingApiStore();
  server.use(...rosterHandlers, ...teachingHandlers);
  // The marks month-read only sees notes/scores of sessions it knows about —
  // register the roster fixtures' sessions so save → re-read round-trips.
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

describe("ClassbookPage stats", () => {
  it("derives the five stat cards from sessions, rosters and enrollments", async () => {
    renderClassbookPage();

    // Rosters resolved: both held sessions show 1/1 present.
    expect(await screen.findAllByText("1/1")).toHaveLength(2);

    const headcount = statCard("SĨ SỐ HIỆN TẠI");
    expect(within(headcount).getByText("1")).toBeInTheDocument();
    expect(within(headcount).getByText("học sinh đang học")).toBeInTheDocument();

    const attendance = statCard("CHUYÊN CẦN THÁNG 8");
    expect(within(attendance).getByText("100%")).toBeInTheDocument();
    expect(within(attendance).getByText("2/2 lượt có mặt")).toBeInTheDocument();

    // The single enrollment spans July and August → full retention.
    const retention = statCard("TÁI TỤC T7→T8");
    expect(within(retention).getByText("100%")).toBeInTheDocument();
    expect(within(retention).getByText("1 → 1 học sinh")).toBeInTheDocument();

    // No scores stored yet — dash, never a fake zero.
    const average = statCard("ĐIỂM TB LỚP");
    expect(within(average).getByText("—")).toBeInTheDocument();
    expect(within(average).getByText("trung bình 0 buổi")).toBeInTheDocument();

    // 2 held × 1 present × 150.000đ = 300.000đ gross − 2 × 300.000đ cost.
    const profit = statCard("LÃI/LỖ THÁNG 8");
    expect(within(profit).getByText("-300.000đ")).toBeInTheDocument();
    expect(within(profit).getByText("thu 300.000đ − chi 600.000đ")).toBeInTheDocument();
  });
});

describe("ClassbookPage sessions table", () => {
  it("renders held, cancelled and planned rows with the month window applied", async () => {
    renderClassbookPage();

    const heldRow = await findHeldRow();
    expect(within(heldRow).getByText("1/1")).toBeInTheDocument();
    expect(within(heldRow).getByText("Chưa nộp")).toBeInTheDocument();
    // 1 present × 150.000đ − 300.000đ session cost.
    expect(within(heldRow).getByText("-150.000đ")).toBeInTheDocument();

    const cancelledRow = screen.getByRole("button", { name: /Th 7, 08\/08/ });
    expect(within(cancelledRow).getByText("buổi hủy")).toBeInTheDocument();
    expect(within(cancelledRow).getByText("Nghỉ lễ")).toBeInTheDocument();

    const plannedRow = screen.getByRole("button", { name: /Th 4, 19\/08/ });
    expect(within(plannedRow).getByText("1 HS")).toBeInTheDocument();
    expect(within(plannedRow).getByText("Chưa nộp")).toBeInTheDocument();

    // Day 26 is after the frozen "today" (Aug 20) — outside the month window.
    expect(screen.queryByText(/26\/08/)).not.toBeInTheDocument();

    expect(
      screen.getByText(/Doanh thu buổi = học phí của học sinh có mặt − 300\.000đ chi phí buổi/),
    ).toBeInTheDocument();
  });

  it("saves a session note from the detail panel and mirrors it in the table", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());

    expect(
      await screen.findByRole("heading", { name: "Buổi Th 4, 05/08 — Toán 6A" }),
    ).toBeInTheDocument();
    expect(screen.getByText("1/1 có mặt")).toBeInTheDocument();

    const textarea = screen.getByLabelText("NHẬN XÉT CHUNG CỦA BUỔI");
    await user.type(textarea, "Lớp sôi nổi, cần ôn phân số");
    expect(screen.getByText("Chưa lưu")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Lưu nhận xét" }));

    expect(
      await screen.findByText("Đã lưu nhận xét buổi Th 4, 05/08 — Toán 6A"),
    ).toBeInTheDocument();
    expect(screen.getByText("Đã lưu ✓")).toBeInTheDocument();
    // Table row shows the saved note too (panel textarea + row cell).
    const heldRow = await findHeldRow();
    expect(within(heldRow).getByText("Lớp sôi nổi, cần ôn phân số")).toBeInTheDocument();
  });

  it("keeps the typed note draft editable when the save fails", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());
    const textarea = await screen.findByLabelText("NHẬN XÉT CHUNG CỦA BUỔI");
    await user.type(textarea, "Bản nháp quan trọng");

    server.use(
      http.put(`${API_URL}/sessions/:id/note`, () =>
        HttpResponse.json({ error: { message: "boom" } }, { status: 500 }),
      ),
    );
    await user.click(screen.getByRole("button", { name: "Lưu nhận xét" }));

    expect(
      await screen.findByText("Không lưu được nhận xét — vui lòng thử lại"),
    ).toBeInTheDocument();
    // No success toast, the draft survives for a retry, and the dirty marker
    // stays — the failed save must not silently discard the user's text.
    expect(screen.queryByText(/Đã lưu nhận xét buổi/)).not.toBeInTheDocument();
    expect(screen.getByLabelText("NHẬN XÉT CHUNG CỦA BUỔI")).toHaveValue("Bản nháp quan trọng");
    expect(screen.getByText("Chưa lưu")).toBeInTheDocument();
  });

  it("saves scores and recomputes the session and class averages", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());
    await user.click(await screen.findByRole("tab", { name: "Điểm buổi" }));

    const input = await screen.findByLabelText("Điểm Nguyễn Văn An");
    await user.type(input, "7.5");
    expect(screen.getByText("Chưa lưu")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Lưu điểm buổi" }));

    expect(
      await screen.findByText("Đã lưu điểm 1 học sinh — buổi Th 4, 05/08"),
    ).toBeInTheDocument();
    const heldRow = await findHeldRow();
    expect(within(heldRow).getByText("7.5")).toBeInTheDocument();
    const average = statCard("ĐIỂM TB LỚP");
    expect(within(average).getByText("7.5")).toBeInTheDocument();
    expect(within(average).getByText("trung bình 1 buổi")).toBeInTheDocument();
  });

  it("shows the empty giáo án state for a session without a plan", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());
    await user.click(await screen.findByRole("tab", { name: "Giáo án" }));

    expect(
      screen.getByText("Chưa có giáo án cho buổi này — soạn ở tab Chương trình & giáo án."),
    ).toBeInTheDocument();
  });
});

describe("ClassbookPage CSV export", () => {
  it("downloads the class CSV with BOM, quoted cells and a toast", async () => {
    const createObjectURL = vi.fn<(blob: Blob) => string>(() => "blob:classbook");
    const revokeObjectURL = vi.fn();
    Object.assign(URL, { createObjectURL, revokeObjectURL });

    const user = userEvent.setup();
    renderClassbookPage();
    // Wait for rosters so the CSV carries the revenue columns.
    await screen.findAllByText("1/1");

    await user.click(screen.getByRole("button", { name: /Tải dữ liệu lớp \(CSV\)/ }));

    expect(await screen.findByText("Đã tải Toán_6A_ky08.csv")).toBeInTheDocument();
    expect(createObjectURL).toHaveBeenCalledTimes(1);
    const blob = createObjectURL.mock.calls[0]![0];
    // Blob.text() decodes as UTF-8 and strips the BOM — check its bytes.
    const bytes = new Uint8Array((await blob.arrayBuffer()).slice(0, 3));
    expect([...bytes]).toEqual([0xef, 0xbb, 0xbf]);
    const text = await blob.text();
    expect(text.startsWith('"Buổi";"Trạng thái";"Bài học"')).toBe(true);
    expect(text).toContain('"Th 4, 05/08";"Đã dạy"');
    expect(text).toContain('"Hủy"');
    expect(text).toContain('"-150000"');
  });
});
