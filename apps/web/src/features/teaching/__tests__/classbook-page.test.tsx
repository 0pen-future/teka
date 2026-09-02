import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import {
  classWithSchedule,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
} from "@/features/roster/__tests__/roster-handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassbookPage } from "../pages/classbook-page";
import {
  resetTeachingApiStore,
  seedScoreComponents,
  seedTeachingSession,
  teachingHandlers,
} from "./teaching-handlers";

/** `classWithSchedule.id` in `roster-handlers.ts` — the only seeded class. */
const CLASS_ID = "70000000-0000-4000-8000-000000000001";

function renderClassbookPage(route = "/classbook") {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ClassbookPage />, { route, path: "/classbook" });
}

/** The KPI tile owning `label` — its value/sub are sibling `<dd>`s. */
function kpi(label: string): HTMLElement {
  return screen.getByText(label).parentElement!;
}

/** The day-5 held session's BUỔI button (its label under the frozen clock). */
async function findHeldRow() {
  return await screen.findByRole("button", { name: /Th 4, 05\/08/ });
}

/** The whole `<tr>` a BUỔI button sits in — the other cells live there. */
function rowOf(button: HTMLElement): HTMLElement {
  return button.closest("tr")!;
}

const heldRegionName = "Chi tiết buổi Th 4, 05/08";

/** Replaces the scores PUT with a counter that acknowledges every save. */
function countScorePuts() {
  const counter = { count: 0 };
  server.use(
    http.put(`${API_URL}/sessions/:id/scores`, () => {
      counter.count += 1;
      return HttpResponse.json(ok([]));
    }),
  );
  return counter;
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
  vi.restoreAllMocks();
});

describe("ClassbookPage KPI strip", () => {
  it("derives the four KPIs from sessions, rosters and enrollments", async () => {
    renderClassbookPage();

    // Rosters resolved: both held sessions show 1/1 present.
    expect(await screen.findAllByText("1/1")).toHaveLength(2);

    const headcount = kpi("SĨ SỐ");
    expect(within(headcount).getByText("1")).toBeInTheDocument();
    // The single enrollment spans July and August → full retention.
    expect(within(headcount).getByText("tái tục 100%")).toBeInTheDocument();

    const attendance = kpi("CHUYÊN CẦN");
    expect(within(attendance).getByText("100%")).toBeInTheDocument();
    expect(within(attendance).getByText("2/2 lượt")).toBeInTheDocument();

    // No scores stored yet — dash, never a fake zero.
    const average = kpi("ĐIỂM TB");
    expect(within(average).getByText("—")).toBeInTheDocument();
    expect(within(average).getByText("0 buổi")).toBeInTheDocument();

    // 2 held × 1 present × 150.000đ = 300.000đ gross − 2 × 300.000đ cost.
    const profit = kpi("LÃI/LỖ T8");
    expect(within(profit).getByText("-300.000đ")).toHaveClass("text-coral-600");
    expect(within(profit).getByText("thu 300.000đ · chi 600.000đ")).toBeInTheDocument();
  });
});

describe("ClassbookPage sessions ledger", () => {
  it("renders held, cancelled and planned rows with the month window applied", async () => {
    renderClassbookPage();

    const heldRow = rowOf(await findHeldRow());
    expect(within(heldRow).getByText("1/1")).toBeInTheDocument();
    expect(within(heldRow).getByText("Chưa nộp")).toBeInTheDocument();
    // 1 present × 150.000đ − 300.000đ session cost.
    expect(within(heldRow).getByText("-150.000đ")).toBeInTheDocument();
    // Work chips: no note yet, nobody scored out of the one present student.
    expect(within(heldRow).getByText("Chưa có")).toBeInTheDocument();
    expect(within(heldRow).getByText("0/1")).toBeInTheDocument();
    // Phone-only VIỆC column names the next chore (hidden by CSS on sm+).
    expect(within(heldRow).getByText("Nhận xét").closest("td")).toHaveClass("sm:hidden");

    const cancelledRow = rowOf(screen.getByRole("button", { name: /Th 7, 08\/08/ }));
    expect(within(cancelledRow).getByText("Nghỉ lễ")).toBeInTheDocument();
    expect(within(cancelledRow).getByText("Buổi hủy")).toBeInTheDocument();

    const plannedRow = rowOf(screen.getByRole("button", { name: /Th 4, 19\/08/ }));
    expect(within(plannedRow).getByText("1 dự kiến")).toBeInTheDocument();
    expect(within(plannedRow).getByText("Chưa nộp")).toBeInTheDocument();

    // Day 26 is after the frozen "today" (Aug 20) — outside the month window.
    expect(screen.queryByText(/26\/08/)).not.toBeInTheDocument();

    expect(
      screen.getByText(/Doanh thu buổi = học phí của học sinh có mặt − 300\.000đ chi phí buổi/),
    ).toBeInTheDocument();
  });

  it("reads the month from the URL and shows the empty month state", async () => {
    renderClassbookPage("/classbook?month=2026-07");

    expect(await screen.findByText("Tháng 7/2026")).toBeInTheDocument();
    expect(await screen.findByText("Chưa có buổi học nào trong tháng 7.")).toBeInTheDocument();
    expect(screen.getByText("LÃI/LỖ T7")).toBeInTheDocument();
  });

  it("steps the month with the stepper", async () => {
    const user = userEvent.setup();
    renderClassbookPage();
    await findHeldRow();

    await user.click(screen.getByRole("button", { name: "Tháng trước" }));
    expect(await screen.findByText("Tháng 7/2026")).toBeInTheDocument();
    expect(await screen.findByText("Chưa có buổi học nào trong tháng 7.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Tháng sau" }));
    expect(await screen.findByText("Tháng 8/2026")).toBeInTheDocument();
    expect(await findHeldRow()).toBeInTheDocument();
  });

  it("explains a class_id that matches no active class and offers the first one", async () => {
    const user = userEvent.setup();
    renderClassbookPage("/classbook?class_id=70000000-0000-4000-8000-00000000dead");

    expect(await screen.findByText("Không tìm thấy lớp")).toBeInTheDocument();
    expect(screen.queryByText("SĨ SỐ")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Mở Toán 6A" }));
    expect(await screen.findByText("SĨ SỐ")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Chọn lớp/ })).toHaveTextContent("Toán 6A");
  });

  it("shows the empty class state when no class is active", async () => {
    getRosterStore().classes.length = 0;
    renderClassbookPage();

    expect(await screen.findByText("Chưa có lớp đang hoạt động")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Chọn lớp/ })).not.toBeInTheDocument();
  });
});

describe("ClassbookPage expanded session row", () => {
  it("saves a session note from the expanded row and flips the row chip", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());
    expect(await screen.findByRole("region", { name: heldRegionName })).toBeInTheDocument();
    expect(within(rowOf(await findHeldRow())).getByText("đang mở")).toBeInTheDocument();

    const textarea = screen.getByLabelText("NHẬN XÉT CHUNG CỦA BUỔI");
    await user.type(textarea, "Lớp sôi nổi, cần ôn phân số");
    expect(screen.getByText("Chưa lưu")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Lưu nhận xét" }));

    expect(
      await screen.findByText("Đã lưu nhận xét buổi Th 4, 05/08 — Toán 6A"),
    ).toBeInTheDocument();
    expect(screen.getByText("Đã lưu ✓")).toBeInTheDocument();
    const heldRow = rowOf(await findHeldRow());
    expect(within(heldRow).getByText("Đã có")).toBeInTheDocument();
    expect(within(heldRow).queryByText("Chưa có")).not.toBeInTheDocument();
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

  it("saves general scores inline and recomputes the session and class averages", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());

    const input = await screen.findByLabelText("Điểm Nguyễn Văn An");
    expect(input).toHaveAttribute("type", "text");
    // Vietnamese decimal comma is accepted and normalised to 7.5 on save.
    await user.type(input, "7,5");
    expect(screen.getByRole("status")).toHaveTextContent("1 ô chưa lưu");

    await user.click(screen.getByRole("button", { name: "Lưu điểm buổi" }));

    expect(
      await screen.findByText("Đã lưu điểm 1 học sinh — buổi Th 4, 05/08"),
    ).toBeInTheDocument();
    const heldRow = rowOf(await findHeldRow());
    expect(within(heldRow).getByText("7,5")).toBeInTheDocument();
    const average = kpi("ĐIỂM TB");
    expect(within(average).getByText("7,5")).toBeInTheDocument();
    expect(within(average).getByText("1 buổi")).toBeInTheDocument();
  });

  it("blocks saving while a general score cannot be parsed", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());

    const input = await screen.findByLabelText("Điểm Nguyễn Văn An");
    await user.type(input, "abc");
    await user.tab();

    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription("Điểm 0–10, bước 0,5");
    expect(screen.getByRole("button", { name: "Lưu điểm buổi" })).toBeDisabled();

    await user.clear(input);
    await user.type(input, "8");
    await user.tab();
    expect(input).not.toHaveAttribute("aria-invalid");
    expect(screen.getByRole("button", { name: "Lưu điểm buổi" })).toBeEnabled();
  });

  it("shows the empty giáo án state for a session without a plan", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());

    expect(
      await screen.findByText("Chưa có giáo án cho buổi này — soạn ở tab Chương trình & giáo án."),
    ).toBeInTheDocument();
  });

  it("closes with the footer button, Escape, and walks rows with the arrow keys", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    const heldButton = await findHeldRow();
    await user.click(heldButton);
    await screen.findByRole("region", { name: heldRegionName });
    await user.click(screen.getByRole("button", { name: "Đóng chi tiết buổi" }));
    expect(screen.queryByRole("region", { name: /Chi tiết buổi/ })).not.toBeInTheDocument();
    expect(heldButton).toHaveFocus();

    await user.keyboard("{Enter}");
    await user.click(await screen.findByLabelText("NHẬN XÉT CHUNG CỦA BUỔI"));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("region", { name: /Chi tiết buổi/ })).not.toBeInTheDocument();
    expect(heldButton).toHaveFocus();

    await user.keyboard("{Enter}");
    await screen.findByRole("region", { name: heldRegionName });
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("region", { name: /Chi tiết buổi/ })).not.toBeInTheDocument();
    expect(heldButton).toHaveFocus();

    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("button", { name: /Th 7, 08\/08/ })).toHaveFocus();
    await user.keyboard("{ArrowUp}");
    expect(heldButton).toHaveFocus();
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

describe("ClassbookPage unsaved-score guards", () => {
  it("saves an unsaved general score through 'Lưu và đóng' before switching sessions", async () => {
    let markPuts = 0;
    server.use(
      http.put(`${API_URL}/sessions/:id/marks`, () => {
        markPuts += 1;
        return HttpResponse.json(ok([]));
      }),
    );
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());
    await user.type(await screen.findByLabelText("Điểm Nguyễn Văn An"), "8");
    await user.click(screen.getByRole("button", { name: /Th 4, 12\/08/ }));

    const guard = await screen.findByRole("dialog", { name: "Còn 1 ô chưa lưu" });
    await user.click(within(guard).getByRole("button", { name: "Lưu và đóng" }));

    expect(
      await screen.findByText("Đã lưu điểm 1 học sinh — buổi Th 4, 05/08"),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("region", { name: "Chi tiết buổi Th 4, 12/08" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(markPuts).toBe(1);
  });

  it("guards an unsaved note and saves it through 'Lưu và đóng'", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());
    await user.type(await screen.findByLabelText("NHẬN XÉT CHUNG CỦA BUỔI"), "Lớp sôi nổi");
    await user.keyboard("{Escape}");

    const guard = await screen.findByRole("dialog", { name: "Còn 1 ô chưa lưu" });
    await user.click(within(guard).getByRole("button", { name: "Lưu và đóng" }));

    expect(
      await screen.findByText("Đã lưu nhận xét buổi Th 4, 05/08 — Toán 6A"),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.queryByRole("region", { name: /Chi tiết buổi/ })).not.toBeInTheDocument(),
    );
    expect(within(rowOf(await findHeldRow())).getByText("Đã có")).toBeInTheDocument();
  });

  async function typeUnsavedComponentScore(user: ReturnType<typeof userEvent.setup>) {
    await user.click(await findHeldRow());
    await user.type(await screen.findByLabelText("Điểm 15 phút Nguyễn Văn An"), "9");
  }

  describe("with component scores", () => {
    beforeEach(() => {
      seedScoreComponents(CLASS_ID, [{ id: "comp-quiz", name: "15 phút", position: 1 }]);
    });

    it("guards switching sessions while component scores are unsaved", async () => {
      const puts = countScorePuts();
      const user = userEvent.setup();
      renderClassbookPage();
      await typeUnsavedComponentScore(user);

      await user.click(screen.getByRole("button", { name: /Th 4, 12\/08/ }));
      const guard = await screen.findByRole("dialog", { name: "Còn 1 ô chưa lưu" });
      await user.click(within(guard).getByRole("button", { name: "Bỏ thay đổi" }));

      expect(
        await screen.findByRole("region", { name: "Chi tiết buổi Th 4, 12/08" }),
      ).toBeInTheDocument();
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(puts.count).toBe(0);
    });

    it("guards switching classes while component scores are unsaved", async () => {
      getRosterStore().classes.push({
        ...classWithSchedule,
        id: "70000000-0000-4000-8000-000000000002",
        name: "Toán 6B",
      });
      const puts = countScorePuts();
      const user = userEvent.setup();
      renderClassbookPage();
      await typeUnsavedComponentScore(user);

      await user.click(screen.getByRole("button", { name: /^Chọn lớp/ }));
      const picker = await screen.findByRole("dialog", { name: /^Chọn lớp/ });
      await user.click(within(picker).getByRole("button", { name: /Toán 6B/ }));

      const guard = await screen.findByRole("dialog", { name: "Còn 1 ô chưa lưu" });
      // The modal hides the page from the a11y tree while it is open.
      expect(screen.getByRole("button", { name: /^Chọn lớp/, hidden: true })).toHaveTextContent(
        "Toán 6A",
      );

      await user.click(within(guard).getByRole("button", { name: "Bỏ thay đổi" }));
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      expect(screen.getByRole("button", { name: /^Chọn lớp/ })).toHaveTextContent("Toán 6B");
      expect(screen.queryByRole("region", { name: /Chi tiết buổi/ })).not.toBeInTheDocument();
      expect(puts.count).toBe(0);
    });

    it("guards stepping the month while component scores are unsaved", async () => {
      const puts = countScorePuts();
      const user = userEvent.setup();
      renderClassbookPage();
      await typeUnsavedComponentScore(user);

      await user.click(screen.getByRole("button", { name: "Tháng trước" }));
      const guard = await screen.findByRole("dialog", { name: "Còn 1 ô chưa lưu" });
      await user.click(within(guard).getByRole("button", { name: "Ở lại" }));
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      expect(screen.getByText("Tháng 8/2026")).toBeInTheDocument();
      expect(screen.getByLabelText("Điểm 15 phút Nguyễn Văn An")).toHaveValue("9");

      await user.click(screen.getByRole("button", { name: "Tháng trước" }));
      const again = await screen.findByRole("dialog", { name: "Còn 1 ô chưa lưu" });
      await user.click(within(again).getByRole("button", { name: "Bỏ thay đổi" }));
      expect(await screen.findByText("Tháng 7/2026")).toBeInTheDocument();
      expect(screen.queryByRole("region", { name: /Chi tiết buổi/ })).not.toBeInTheDocument();
      expect(puts.count).toBe(0);
    });
  });
});
