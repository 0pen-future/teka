import { screen } from "@testing-library/react";
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
// Deliberately out of position order to prove the grid re-sorts columns.
const COMPONENT_MIENG = { id: "comp-mieng", name: "Miệng", position: 2 };
const COMPONENT_15P = { id: "comp-15p", name: "15 phút", position: 1 };

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

beforeEach(() => {
  // Same frozen clock as classbook-page.test.tsx so the fixture sessions and
  // the month window line up.
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

describe("scores tab — class without score components", () => {
  it("renders the plain general-score block unchanged and never the grid", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());
    await user.click(await screen.findByRole("tab", { name: "Điểm buổi" }));

    expect(await screen.findByLabelText("Điểm Nguyễn Văn An")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Lưu điểm buổi" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Lưu điểm thành phần" })).not.toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });
});

describe("scores tab — class with score components", () => {
  beforeEach(() => {
    seedScoreComponents(CLASS_ID, [COMPONENT_MIENG, COMPONENT_15P]);
  });

  it("replaces the general-score block with the component grid, columns in position order", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findHeldRow());
    await user.click(await screen.findByRole("tab", { name: "Điểm buổi" }));

    const headers = await screen.findAllByRole("columnheader");
    expect(headers.map((header) => header.textContent)).toEqual(["Học sinh", "15 phút", "Miệng"]);
    expect(screen.queryByLabelText("Điểm Nguyễn Văn An")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Lưu điểm buổi" })).not.toBeInTheDocument();
    expect(screen.getByText(/điểm thành phần chưa vào báo cáo phụ huynh/i)).toBeInTheDocument();
  });

  it("saves edited cells as a batch PUT and a cleared cell round-trips as null", async () => {
    const user = userEvent.setup();
    seedComponentScore("session-05", studentSiblingOne.id, COMPONENT_MIENG.id, 8);
    renderClassbookPage();

    await user.click(await findHeldRow());
    await user.click(await screen.findByRole("tab", { name: "Điểm buổi" }));

    const fifteenMinInput = await screen.findByLabelText("Điểm 15 phút Nguyễn Văn An");
    await user.type(fifteenMinInput, "9");
    const miengInput = screen.getByLabelText("Điểm Miệng Nguyễn Văn An");
    expect(miengInput).toHaveValue(8);
    await user.clear(miengInput);
    expect(screen.getByText("Chưa lưu")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Lưu điểm thành phần" }));

    expect(
      await screen.findByText("Đã lưu điểm thành phần (2 ô) — buổi Th 4, 05/08"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Chưa lưu")).not.toBeInTheDocument();
    const store = getTeachingApiStore();
    expect(
      store.componentScores.get(`session-05#${studentSiblingOne.id}#${COMPONENT_15P.id}`),
    ).toBe(9);
    // The stateful handler only accepts a bare array body (`for (const entry
    // of entries)`) — this round-trip only succeeds end to end because the
    // client actually sent that shape, and a `score: null` request deleted
    // the cell rather than leaving a stray `NaN`/0.
    expect(
      store.componentScores.has(`session-05#${studentSiblingOne.id}#${COMPONENT_MIENG.id}`),
    ).toBe(false);
    // And the cleared cell must stay empty in the grid: the PUT echoes the
    // session's full current set (a cleared cell simply absent), so onSuccess
    // replaces the cache wholesale. A delta-merge would leave the old 8 in
    // cache and repopulate the input on the next render.
    expect(screen.getByLabelText("Điểm Miệng Nguyễn Văn An")).toHaveValue(null);
  });

  it("keeps cells read-only for a not-held session", async () => {
    const user = userEvent.setup();
    renderClassbookPage();

    await user.click(await findPlannedRow());
    await user.click(await screen.findByRole("tab", { name: "Điểm buổi" }));

    await screen.findAllByRole("columnheader");
    expect(screen.queryByLabelText("Điểm 15 phút Nguyễn Văn An")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Điểm Miệng Nguyễn Văn An")).not.toBeInTheDocument();
    expect(screen.getAllByText("—").length).toBeGreaterThan(0);
  });

  it("keeps cells read-only for an absent student on a held session", async () => {
    const user = userEvent.setup();
    getRosterStore().absences["session-05"] = [studentSiblingOne.id];
    renderClassbookPage();

    await user.click(await findHeldRow());
    await user.click(await screen.findByRole("tab", { name: "Điểm buổi" }));

    await screen.findAllByRole("columnheader");
    expect(screen.queryByLabelText("Điểm 15 phút Nguyễn Văn An")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Điểm Miệng Nguyễn Văn An")).not.toBeInTheDocument();
    expect(screen.getAllByText("Vắng").length).toBeGreaterThan(0);
  });
});
