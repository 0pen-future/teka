import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { pickOption } from "@/test/pick-option";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";
import { mockViewport } from "@/test/viewport";

import { ClassTeamSection } from "../components/class-team-section";
import {
  classWithSchedule,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
  staffCandidateHocVu,
  staffCandidateTroGiang,
} from "./roster-handlers";

/** The center directory the invite picker reads: the owner (signed in) plus two members. */
const directoryHandler = http.get(`${API_URL}/centers/me/members/directory`, () =>
  HttpResponse.json(
    ok([
      { teacher_id: testPrimaryTeacher.id, display_name: "Cô Lan", role_name: "Chủ trung tâm" },
      {
        teacher_id: staffCandidateHocVu.id,
        display_name: staffCandidateHocVu.full_name,
        role_name: "Giáo viên",
      },
      {
        teacher_id: staffCandidateTroGiang.id,
        display_name: staffCandidateTroGiang.full_name,
        role_name: "Trợ giảng",
      },
    ]),
  ),
);

const memberCenterHandler = http.get(`${API_URL}/centers/me`, () =>
  HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions: ["classes.read"] })),
);

function renderSection(isOwner: boolean) {
  return renderWithProviders(<ClassTeamSection klass={classWithSchedule} isOwner={isOwner} />);
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers, directoryHandler);
  signInAs(testPrimaryTeacher);
  mockViewport(1024);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("ClassTeamSection", () => {
  it("lists active staff and, for the owner, the open invitations by stage", async () => {
    renderSection(true);

    const team = await screen.findByRole("region", { name: "Đội ngũ giảng dạy" });
    expect(await within(team).findByText("Cô Lan")).toBeInTheDocument();
    expect(within(team).getByText("Giáo viên")).toBeInTheDocument();

    const pending = await within(team).findByRole("listitem", { name: /Cô Hương/ });
    expect(within(pending).getByText("Chờ nhận")).toBeInTheDocument();
    const accepted = within(team).getByRole("listitem", { name: /Thầy Nam/ });
    expect(within(accepted).getByText("Đã đồng ý — chờ phân công")).toBeInTheDocument();
    expect(within(team).getByRole("link", { name: "Xem lời mời" })).toHaveAttribute(
      "href",
      "/class-invitations",
    );
  });

  it("hides the invite button and the invitation list from a member", async () => {
    server.use(memberCenterHandler);
    renderSection(false);

    const team = await screen.findByRole("region", { name: "Đội ngũ giảng dạy" });
    expect(await within(team).findByText("Cô Lan")).toBeInTheDocument();
    expect(within(team).queryByRole("button", { name: "+ Mời GV" })).not.toBeInTheDocument();
    expect(within(team).queryByText("Chờ nhận")).not.toBeInTheDocument();
  });

  it("owner invites a member with a role and a note; the row appears as Chờ nhận", async () => {
    const user = userEvent.setup();
    getRosterStore().classInvitations = [];
    let body: Record<string, unknown> | undefined;
    server.use(
      http.post(`${API_URL}/classes/:classId/invitations`, async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(
          ok({
            id: "a0000000-0000-4000-8000-000000000009",
            class_id: classWithSchedule.id,
            class_name: classWithSchedule.name,
            teacher_id: staffCandidateHocVu.id,
            teacher_name: staffCandidateHocVu.full_name,
            role_key: "hoc_vu",
            role_label: "Học vụ",
            status: "pending",
            invited_by: testPrimaryTeacher.id,
            invited_by_name: "Cô Lan",
            message: "Nhờ thầy theo dõi lớp.",
            sent_at: "2026-09-23T08:00:00Z",
            reminded_at: null,
            responded_at: null,
            assigned_at: null,
          }),
          { status: 201 },
        );
      }),
      http.get(`${API_URL}/class-invitations`, () =>
        HttpResponse.json(
          ok(
            body
              ? [
                  {
                    id: "a0000000-0000-4000-8000-000000000009",
                    class_id: classWithSchedule.id,
                    class_name: classWithSchedule.name,
                    teacher_id: staffCandidateHocVu.id,
                    teacher_name: staffCandidateHocVu.full_name,
                    role_key: "hoc_vu",
                    role_label: "Học vụ",
                    status: "pending",
                    invited_by: testPrimaryTeacher.id,
                    invited_by_name: "Cô Lan",
                    message: "Nhờ thầy theo dõi lớp.",
                    sent_at: "2026-09-23T08:00:00Z",
                    reminded_at: null,
                    responded_at: null,
                    assigned_at: null,
                  },
                ]
              : [],
          ),
        ),
      ),
    );
    renderSection(true);
    const team = await screen.findByRole("region", { name: "Đội ngũ giảng dạy" });
    await within(team).findByText("Cô Lan");

    await user.click(within(team).getByRole("button", { name: "+ Mời GV" }));
    const dialog = await screen.findByRole("dialog", { name: "Mời thành viên vào lớp" });
    // The signed-in owner and the class's active staff are not offered.
    await pickOption(user, within(dialog), /Thành viên/, /Thầy Nam/);
    await pickOption(user, within(dialog), /Vai trò/, "Học vụ");
    await user.type(within(dialog).getByLabelText("Lời nhắn"), "Nhờ thầy theo dõi lớp.");
    await user.click(within(dialog).getByRole("button", { name: "Gửi lời mời" }));

    expect(await screen.findByText("Đã gửi lời mời cho Thầy Nam")).toBeInTheDocument();
    expect(body).toEqual({
      teacher_id: staffCandidateHocVu.id,
      role_key: "hoc_vu",
      message: "Nhờ thầy theo dõi lớp.",
    });
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    const row = await within(team).findByRole("listitem", { name: /Thầy Nam/ });
    expect(within(row).getByText("Chờ nhận")).toBeInTheDocument();
  });

  it("keeps the dialog open and shows the API message when the send is refused", async () => {
    const user = userEvent.setup();
    getRosterStore().classInvitations = [];
    server.use(
      http.post(`${API_URL}/classes/:classId/invitations`, () =>
        HttpResponse.json(
          { error: { code: "CONFLICT", message: "thành viên này đã có lời mời đang chờ" } },
          { status: 409 },
        ),
      ),
    );
    renderSection(true);
    const team = await screen.findByRole("region", { name: "Đội ngũ giảng dạy" });
    await within(team).findByText("Cô Lan");

    await user.click(within(team).getByRole("button", { name: "+ Mời GV" }));
    const dialog = await screen.findByRole("dialog", { name: "Mời thành viên vào lớp" });
    await pickOption(user, within(dialog), /Thành viên/, /Cô Hương/);
    await user.click(within(dialog).getByRole("button", { name: "Gửi lời mời" }));

    expect(
      await within(dialog).findByText("thành viên này đã có lời mời đang chờ"),
    ).toBeInTheDocument();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
