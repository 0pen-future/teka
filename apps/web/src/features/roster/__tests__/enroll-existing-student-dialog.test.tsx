import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { EnrollExistingStudentDialog } from "../components/enroll-existing-student-dialog";
import {
  classWithSchedule,
  contactSingleChild,
  getRosterStore,
  resetRosterStore,
  rosterHandlers,
  studentOnlyChild,
} from "./roster-handlers";

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("EnrollExistingStudentDialog", () => {
  it("enrolls an existing student found by name, without ever showing a phone", async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <EnrollExistingStudentDialog open onOpenChange={() => undefined} klass={classWithSchedule} />,
    );

    // Under two characters the picker explains itself instead of searching.
    expect(screen.getByText("Nhập ít nhất 2 ký tự để tìm theo tên.")).toBeInTheDocument();

    await user.type(screen.getByRole("combobox", { name: "Học sinh" }), "khôi");
    const option = await screen.findByRole("option", { name: studentOnlyChild.full_name });
    // The picker is names-only: no phone and no contact name anywhere in it.
    expect(screen.queryByText(contactSingleChild.phone)).not.toBeInTheDocument();
    expect(screen.queryByText(contactSingleChild.full_name)).not.toBeInTheDocument();
    await user.click(option);

    // Price is inherited from the class and only displayed, never entered.
    expect(screen.getByText("150.000 ₫/buổi")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Ghi danh vào lớp" }));

    expect(
      await screen.findByText(
        new RegExp(
          `Đã ghi danh ${studentOnlyChild.full_name} vào ${classWithSchedule.name} — tính tiền từ buổi có mặt đầu tiên`,
        ),
      ),
    ).toBeInTheDocument();
    expect(
      getRosterStore().enrollments.some(
        (enrollment) =>
          enrollment.student_id === studentOnlyChild.id &&
          enrollment.class_id === classWithSchedule.id,
      ),
    ).toBe(true);
  });
});
