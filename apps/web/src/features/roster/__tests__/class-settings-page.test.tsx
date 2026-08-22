import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassSettingsPage } from "../pages/class-settings-page";
import {
  classSchedule,
  classWithSchedule,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
} from "./roster-handlers";

/** The page's stat card only needs the month's session list; an empty one is fine. */
const sessionsHandler = http.get(`${API_URL}/classes/:classId/sessions`, () =>
  HttpResponse.json(ok([])),
);

/** Matches the page's `today()` (UTC ISO date). */
const todayIso = () => new Date().toISOString().slice(0, 10);

function dayBefore(date: string): string {
  const parsed = new Date(`${date}T00:00:00`);
  parsed.setDate(parsed.getDate() - 1);
  return `${parsed.getFullYear()}-${String(parsed.getMonth() + 1).padStart(2, "0")}-${String(
    parsed.getDate(),
  ).padStart(2, "0")}`;
}

function renderClassSettings() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ClassSettingsPage />, {
    route: `/classes/${classWithSchedule.id}/settings`,
    path: "/classes/:id/settings",
    extraRoutes: [{ path: "/students", element: <div>students-screen-stub</div> }],
  });
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers, sessionsHandler);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("ClassSettingsPage", () => {
  it("renders the classCfg screen prefilled from the class", async () => {
    renderClassSettings();

    expect(
      await screen.findByRole("heading", { name: `Cài đặt lớp — ${classWithSchedule.name}` }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Thay đổi áp dụng từ buổi kế tiếp — các kỳ đã chốt không đổi."),
    ).toBeInTheDocument();
    expect(screen.getByText("HỌC SINH")).toBeInTheDocument();
    expect(screen.getByText("ĐƠN GIÁ HIỆN TẠI")).toBeInTheDocument();

    expect(screen.getByLabelText("Tên lớp")).toHaveValue(classWithSchedule.name);
    expect(screen.getByLabelText("Giờ học khung 1")).toHaveValue("18:00");
    // classSchedule.weekday = 2 → the T3 chip starts selected.
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "T3" })).toHaveAttribute("aria-pressed", "true"),
    );
  });

  it("rejects an empty weekday selection", async () => {
    const user = userEvent.setup();
    renderClassSettings();

    const chip = await screen.findByRole("button", { name: "T3" });
    await waitFor(() => expect(chip).toHaveAttribute("aria-pressed", "true"));
    await user.click(chip);
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    expect(
      await screen.findByText("Mỗi khung giờ cần ít nhất một ngày trong tuần"),
    ).toBeInTheDocument();
  });

  it("warns when the unit price differs from the saved one", async () => {
    const user = userEvent.setup();
    renderClassSettings();

    await screen.findByLabelText("Tên lớp");
    expect(screen.queryByText(/Đơn giá mới chỉ áp cho lượt ghi danh/)).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Tăng 5.000 đồng/ }));

    expect(await screen.findByText(/Đơn giá mới chỉ áp cho lượt ghi danh/)).toBeInTheDocument();
  });

  it("saves name/price plus the schedule diff and returns to the roster", async () => {
    const user = userEvent.setup();
    renderClassSettings();

    const nameInput = await screen.findByLabelText("Tên lớp");
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "T3" })).toHaveAttribute("aria-pressed", "true"),
    );
    await user.clear(nameInput);
    await user.type(nameInput, "Toán 6A nâng cao");
    // Add T7 alongside the existing T3 slot.
    await user.click(screen.getByRole("button", { name: "T7" }));
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    expect(await screen.findByText("students-screen-stub")).toBeInTheDocument();

    const saved = getRosterStore().classes.find((klass) => klass.id === classWithSchedule.id);
    expect(saved?.name).toBe("Toán 6A nâng cao");
    // The unchanged T3 row survives untouched; T7 was added from the next session on.
    expect(saved?.schedules.map((s) => s.weekday).sort()).toEqual([2, 6]);
    const kept = saved?.schedules.find((s) => s.weekday === 2);
    expect(kept?.effective_to).toBeNull();
    const added = saved?.schedules.find((s) => s.weekday === 6);
    expect(added?.start_time).toBe(classSchedule.start_time);
    expect(added?.effective_from).toBe(todayIso());
  });

  it("adds a second khung giờ and saves rows for both times", async () => {
    const user = userEvent.setup();
    renderClassSettings();

    await screen.findByLabelText("Tên lớp");
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "T3" })).toHaveAttribute("aria-pressed", "true"),
    );
    await user.click(screen.getByRole("button", { name: "+ Thêm khung giờ khác" }));

    const secondTime = await screen.findByLabelText("Giờ học khung 2");
    await user.clear(secondTime);
    await user.type(secondTime, "20:00");
    // The new slot meets on T7 — its chips render after the first slot's, so
    // the last "T7" button on screen belongs to khung giờ 2.
    const chips = screen.getAllByRole("button", { name: "T7" });
    expect(chips).toHaveLength(2);
    await user.click(chips[1]!);
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    expect(await screen.findByText("students-screen-stub")).toBeInTheDocument();

    const saved = getRosterStore().classes.find((klass) => klass.id === classWithSchedule.id);
    expect(saved?.schedules).toHaveLength(2);
    const kept = saved?.schedules.find((s) => s.id === classSchedule.id);
    expect(kept?.effective_to).toBeNull();
    const added = saved?.schedules.find((s) => s.id !== classSchedule.id);
    expect(added?.weekday).toBe(6);
    expect(added?.start_time).toBe("20:00");
    expect(added?.effective_from).toBe(todayIso());
  });

  it("rejects a weekday that already belongs to another khung giờ", async () => {
    const user = userEvent.setup();
    renderClassSettings();

    await screen.findByLabelText("Tên lớp");
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "T3" })).toHaveAttribute("aria-pressed", "true"),
    );
    await user.click(screen.getByRole("button", { name: "+ Thêm khung giờ khác" }));

    // Slot 1 already meets on T3 — picking T3 again in slot 2 must not save:
    // the backend generates at most one session per class per date.
    const chips = screen.getAllByRole("button", { name: "T3" });
    expect(chips).toHaveLength(2);
    await user.click(chips[1]!);
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    expect(
      await screen.findByText("Ngày này đã có ở khung giờ khác — mỗi ngày chỉ một khung giờ"),
    ).toBeInTheDocument();
    expect(screen.queryByText("students-screen-stub")).not.toBeInTheDocument();
  });

  it("closes the old row instead of deleting it when the time changes", async () => {
    const user = userEvent.setup();
    renderClassSettings();

    const timeInput = await screen.findByLabelText("Giờ học khung 1");
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "T3" })).toHaveAttribute("aria-pressed", "true"),
    );
    await user.clear(timeInput);
    await user.type(timeInput, "19:30");
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    expect(await screen.findByText("students-screen-stub")).toBeInTheDocument();

    const saved = getRosterStore().classes.find((klass) => klass.id === classWithSchedule.id);
    // Close-and-replace: the old T3 row stays, closed yesterday, so past
    // sessions remain explicable; the new row starts today.
    expect(saved?.schedules).toHaveLength(2);
    const closed = saved?.schedules.find((s) => s.id === classSchedule.id);
    expect(closed?.start_time).toBe(classSchedule.start_time);
    expect(closed?.effective_to).toBe(dayBefore(todayIso()));
    const replacement = saved?.schedules.find((s) => s.id !== classSchedule.id);
    expect(replacement?.weekday).toBe(classSchedule.weekday);
    expect(replacement?.start_time).toBe("19:30");
    expect(replacement?.effective_from).toBe(todayIso());
  });
});
