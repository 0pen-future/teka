import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { EnrollStudentDialog } from "../components/enroll-student-dialog";
import {
  classWithSchedule,
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

describe("EnrollStudentDialog", () => {
  it("surfaces a next-session billing note after a successful enrollment", async () => {
    const user = userEvent.setup();
    const today = new Date().toISOString().slice(0, 10);
    renderWithProviders(
      <EnrollStudentDialog
        open
        onOpenChange={() => undefined}
        mode="class"
        classId={classWithSchedule.id}
      />,
    );

    // studentOnlyChild is not yet enrolled in classWithSchedule, so it's a
    // valid pick from the "search a student for this class" search list.
    const searchInput = await screen.findByRole("combobox", { name: "Tìm học sinh" });
    await user.type(searchInput, studentOnlyChild.full_name);
    const option = await screen.findByRole("option", {
      name: new RegExp(studentOnlyChild.full_name),
    });
    await user.click(option);

    await user.click(screen.getByRole("button", { name: "Ghi danh" }));

    expect(
      await screen.findByText(
        new RegExp(
          `Học phí của ${studentOnlyChild.full_name} được tính từ buổi học tiếp theo kể từ ${today}`,
        ),
      ),
    ).toBeInTheDocument();
  });

  it("shows the class's inherited, read-only unit price for a fixed class", async () => {
    renderWithProviders(
      <EnrollStudentDialog
        open
        onOpenChange={() => undefined}
        mode="class"
        classId={classWithSchedule.id}
      />,
    );

    expect(
      await screen.findByText(/Đơn giá kế thừa từ lớp, V1 không sửa được\./),
    ).toBeInTheDocument();
  });
});
