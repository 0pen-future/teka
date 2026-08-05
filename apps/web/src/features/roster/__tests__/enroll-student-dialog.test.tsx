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

  it("enrolls a fixed student through the class picker with the inherited price", async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <EnrollStudentDialog
        open
        onOpenChange={() => undefined}
        mode="student"
        studentId={studentOnlyChild.id}
        stepBadge="Bước 2/2"
        onLater={() => undefined}
      />,
    );

    // The wizard's second step echoes the just-created student back and
    // offers the postpone path instead of a plain cancel.
    expect(await screen.findByText(studentOnlyChild.full_name)).toBeInTheDocument();
    expect(screen.getByText(studentOnlyChild.contact_name)).toBeInTheDocument();
    expect(screen.getByText("Bước 2/2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Để sau" })).toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: "Lớp" }));
    await user.click(await screen.findByRole("option", { name: /Toán 6A/ }));

    // Price is inherited from the class and only displayed, never entered.
    expect(screen.getByText("150.000 ₫/buổi")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Ghi danh vào lớp" }));

    expect(
      await screen.findByText(
        new RegExp(
          `Đã ghi danh ${studentOnlyChild.full_name} vào Toán 6A — tính tiền từ buổi có mặt đầu tiên`,
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
