import { screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassDetailPage } from "../pages/class-detail-page";
import {
  classSchedule,
  classWithSchedule,
  resetRosterStore,
  rosterHandlers,
} from "./roster-handlers";

function renderClassDetail() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ClassDetailPage />, {
    route: `/classes/${classWithSchedule.id}`,
    path: "/classes/:id",
  });
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("ClassDetailPage", () => {
  it("renders one row per weekly schedule entry", async () => {
    renderClassDetail();

    expect(
      await screen.findByText(
        `Thứ 3 · ${classSchedule.start_time} · ${classSchedule.duration_min} phút`,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(`Áp dụng từ ${classSchedule.effective_from}`)).toBeInTheDocument();
  });

  it("lists enrolled students alongside their enroll/end actions", async () => {
    renderClassDetail();

    expect(await screen.findByText("Học sinh trong lớp")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Kết thúc ghi danh" })).toBeInTheDocument();
  });
});
