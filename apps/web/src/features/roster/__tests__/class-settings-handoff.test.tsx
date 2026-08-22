import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassSettingsPage } from "../pages/class-settings-page";
import { classWithSchedule, resetRosterStore, rosterHandlers } from "./roster-handlers";

/** The stat card only needs the month's session list; an empty one is fine. */
const sessionsHandler = http.get(`${API_URL}/classes/:classId/sessions`, () =>
  HttpResponse.json(ok([])),
);

/** The class's current teacher plus one other member to hand it to. */
const CURRENT_TEACHER = classWithSchedule.teacher_id;
const TARGET_TEACHER = "73000000-0000-4000-8000-000000000002";

/** Owner body where the signed-in owner also teaches the fixture class. */
const ownerCenterHandler = http.get(`${API_URL}/centers/me`, () =>
  HttpResponse.json(
    ok({
      center: { id: "30000000-0000-4000-8000-000000000001", name: "Trung Tâm Bình Minh", is_owner: true },
      members: [
        { id: CURRENT_TEACHER, full_name: "Cô Lan", phone: "+84901000001", is_owner: true },
        { id: TARGET_TEACHER, full_name: "Thầy Nam", phone: "+84901000002", is_owner: false },
      ],
    }),
  ),
);

/** Member body — no roster, so the settings page must not render the card. */
const memberCenterHandler = http.get(`${API_URL}/centers/me`, () =>
  HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
);

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

describe("ClassSettingsPage teacher handoff", () => {
  it("shows the current teacher and only other members as targets, for owners", async () => {
    server.use(ownerCenterHandler);
    renderClassSettings();

    expect(await screen.findByText("Giáo viên phụ trách")).toBeInTheDocument();
    // Current teacher named; the select offers the other member, not the current one.
    expect(await screen.findByText("Cô Lan")).toBeInTheDocument();
    const select = screen.getByLabelText("Bàn giao cho");
    expect(within(select).queryByRole("option", { name: /Cô Lan/ })).not.toBeInTheDocument();
    expect(within(select).getByRole("option", { name: /Thầy Nam/ })).toBeInTheDocument();
  });

  it("does not render the section for member accounts", async () => {
    server.use(memberCenterHandler);
    renderClassSettings();

    // The settings form still loads; only the owner-only card is absent.
    await screen.findByLabelText("Tên lớp");
    expect(screen.queryByText("Giáo viên phụ trách")).not.toBeInTheDocument();
  });

  it("hands the class to the picked member after a two-click confirm", async () => {
    const user = userEvent.setup();
    server.use(ownerCenterHandler);
    renderClassSettings();

    const select = await screen.findByLabelText("Bàn giao cho");
    await user.selectOptions(select, TARGET_TEACHER);
    await user.click(screen.getByRole("button", { name: "Bàn giao lớp" }));
    // Arming reveals the explicit confirm; nothing is sent yet.
    await user.click(screen.getByRole("button", { name: "Xác nhận bàn giao" }));

    expect(await screen.findByText("Đã bàn giao lớp cho Thầy Nam")).toBeInTheDocument();
  });

  it("surfaces a 422 from the API inline", async () => {
    const user = userEvent.setup();
    server.use(
      ownerCenterHandler,
      http.put(`${API_URL}/classes/:id/teacher`, () =>
        HttpResponse.json(fail("VALIDATION_ERROR", "giáo viên này không thuộc trung tâm của bạn"), {
          status: 422,
        }),
      ),
    );
    renderClassSettings();

    const select = await screen.findByLabelText("Bàn giao cho");
    await user.selectOptions(select, TARGET_TEACHER);
    await user.click(screen.getByRole("button", { name: "Bàn giao lớp" }));
    await user.click(screen.getByRole("button", { name: "Xác nhận bàn giao" }));

    expect(
      await screen.findByText("giáo viên này không thuộc trung tâm của bạn"),
    ).toBeInTheDocument();
  });
});
