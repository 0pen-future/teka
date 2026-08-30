import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { centerKeys } from "@/features/center";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { ClassStaffSection } from "../components/class-staff-section";
import {
  classWithSchedule,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
  staffCandidateHocVu,
  staffCandidateTroGiang,
} from "./roster-handlers";

const CURRENT_TEACHER = classWithSchedule.teacher_id;

/** Owner body: the current teacher plus the two staff candidates. */
const ownerCenterHandler = http.get(`${API_URL}/centers/me`, () =>
  HttpResponse.json(
    ok({
      center: {
        id: "30000000-0000-4000-8000-000000000001",
        name: "Trung Tâm Bình Minh",
        is_owner: true,
      },
      members: [
        { id: CURRENT_TEACHER, full_name: "Cô Lan", phone: "+84901000001", is_owner: true },
        {
          id: staffCandidateHocVu.id,
          full_name: staffCandidateHocVu.full_name,
          phone: "+84901000002",
          is_owner: false,
        },
        {
          id: staffCandidateTroGiang.id,
          full_name: staffCandidateTroGiang.full_name,
          phone: "+84901000003",
          is_owner: false,
        },
      ],
      permissions: [],
    }),
  ),
);

/** Member body — no roster, so the section must not render at all. */
const memberCenterHandler = http.get(`${API_URL}/centers/me`, () =>
  HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions: [] })),
);

function renderSection() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<ClassStaffSection classId={classWithSchedule.id} />);
}

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("ClassStaffSection", () => {
  it("lists staff by role, giao_vien read-only with a link to the handoff card", async () => {
    server.use(ownerCenterHandler);
    renderSection();

    expect(await screen.findByText("Nhân sự lớp")).toBeInTheDocument();
    expect(await screen.findByText("Cô Lan")).toBeInTheDocument();
    expect(screen.getByText("Giáo viên")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Bàn giao lớp" })).toHaveAttribute(
      "href",
      "#teacher-handoff",
    );
    // The giao_vien row shows no "Gỡ" — only the two "+ Thêm ..." group triggers exist.
    expect(screen.queryAllByRole("button", { name: "Gỡ" })).toHaveLength(0);
  });

  it("badges every role with no active assignment yet", async () => {
    server.use(ownerCenterHandler);
    renderSection();

    await screen.findByText("Nhân sự lớp");
    expect(await screen.findByText("Thiếu Học vụ")).toBeInTheDocument();
    expect(screen.getByText("Thiếu Trợ giảng")).toBeInTheDocument();
    // giao_vien is seeded active — no badge for it.
    expect(screen.queryByText("Thiếu Giáo viên")).not.toBeInTheDocument();
  });

  it("hides entirely for a non-owner account and never requests the staff list", async () => {
    let staffRequested = false;
    server.use(
      memberCenterHandler,
      http.get(`${API_URL}/classes/:classId/staff`, () => {
        staffRequested = true;
        return HttpResponse.json(ok([]));
      }),
    );
    const { queryClient } = renderSection();

    await waitFor(() => expect(queryClient.getQueryState(centerKeys.me)?.status).toBe("success"));
    expect(screen.queryByText("Nhân sự lớp")).not.toBeInTheDocument();
    expect(staffRequested).toBe(false);
  });

  it("assigns a hoc_vu member and clears the missing-role badge for it", async () => {
    const user = userEvent.setup();
    server.use(ownerCenterHandler);
    renderSection();

    await screen.findByText("Thiếu Học vụ");
    await user.click(screen.getByRole("button", { name: "+ Thêm học vụ" }));
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Chọn học vụ" }),
      staffCandidateHocVu.id,
    );
    await user.click(screen.getByRole("button", { name: "Xác nhận" }));

    expect(await screen.findByText(staffCandidateHocVu.full_name)).toBeInTheDocument();
    expect(screen.queryByText("Thiếu Học vụ")).not.toBeInTheDocument();
  });

  it("soft-closes a staff role by default, keeping the row with ended_at set", async () => {
    const user = userEvent.setup();
    server.use(ownerCenterHandler);
    renderSection();

    await screen.findByText("Thiếu Học vụ");
    await user.click(screen.getByRole("button", { name: "+ Thêm học vụ" }));
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Chọn học vụ" }),
      staffCandidateHocVu.id,
    );
    await user.click(screen.getByRole("button", { name: "Xác nhận" }));
    await screen.findByText(staffCandidateHocVu.full_name);

    await user.click(screen.getByRole("button", { name: "Gỡ" }));
    await user.click(screen.getByRole("button", { name: "Kết thúc vai trò" }));

    expect(await screen.findByText("Thiếu Học vụ")).toBeInTheDocument();
    const closed = getRosterStore().classStaff.find(
      (item) => item.teacher_id === staffCandidateHocVu.id,
    );
    expect(closed).toBeDefined();
    expect(closed?.ended_at).not.toBeNull();
  });

  it("requires a second confirm for void and hard-deletes the row (mode=void)", async () => {
    const user = userEvent.setup();
    server.use(ownerCenterHandler);
    renderSection();

    await screen.findByText("Thiếu Học vụ");
    await user.click(screen.getByRole("button", { name: "+ Thêm học vụ" }));
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Chọn học vụ" }),
      staffCandidateHocVu.id,
    );
    await user.click(screen.getByRole("button", { name: "Xác nhận" }));
    await screen.findByText(staffCandidateHocVu.full_name);

    await user.click(screen.getByRole("button", { name: "Gỡ" }));
    await user.click(screen.getByRole("button", { name: "Gán nhầm — thu hồi" }));
    // First click only arms; nothing is sent yet.
    expect(
      getRosterStore().classStaff.some((item) => item.teacher_id === staffCandidateHocVu.id),
    ).toBe(true);
    await user.click(screen.getByRole("button", { name: "Xác nhận thu hồi" }));

    expect(await screen.findByText("Thiếu Học vụ")).toBeInTheDocument();
    expect(
      getRosterStore().classStaff.some((item) => item.teacher_id === staffCandidateHocVu.id),
    ).toBe(false);
  });

  it("reveals a soft-closed stint in the ended list and void-revokes it from there", async () => {
    const user = userEvent.setup();
    server.use(ownerCenterHandler);
    renderSection();

    // Soft-close hoc_vu first so there is an ended stint to reveal.
    await screen.findByText("Thiếu Học vụ");
    await user.click(screen.getByRole("button", { name: "+ Thêm học vụ" }));
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Chọn học vụ" }),
      staffCandidateHocVu.id,
    );
    await user.click(screen.getByRole("button", { name: "Xác nhận" }));
    await screen.findByText(staffCandidateHocVu.full_name);
    await user.click(screen.getByRole("button", { name: "Gỡ" }));
    await user.click(screen.getByRole("button", { name: "Kết thúc vai trò" }));
    await screen.findByText("Thiếu Học vụ");

    // Collapsed by default.
    expect(screen.queryByText(/kết thúc\s+\d/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Hiện đã kết thúc (1)" }));
    expect(
      screen.getByText(new RegExp(`${staffCandidateHocVu.full_name}.*Học vụ.*kết thúc`)),
    ).toBeInTheDocument();

    // "Thu hồi" opens straight into the armed void step — one more click hard-deletes.
    await user.click(screen.getByRole("button", { name: "Thu hồi" }));
    expect(screen.getByRole("button", { name: "Xác nhận thu hồi" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Kết thúc vai trò" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Xác nhận thu hồi" }));

    await waitFor(() =>
      expect(
        getRosterStore().classStaff.some((item) => item.teacher_id === staffCandidateHocVu.id),
      ).toBe(false),
    );
    expect(screen.queryByRole("button", { name: /Hiện đã kết thúc/ })).not.toBeInTheDocument();
  });
});
