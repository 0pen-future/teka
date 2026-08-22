import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { EnrollStudentDialog } from "../components/enroll-student-dialog";
import { resetRosterStore, rosterHandlers, studentOnlyChild } from "./roster-handlers";

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("EnrollStudentDialog", () => {
  it("enrolls a fixed student through the class picker with the inherited price", async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <EnrollStudentDialog
        open
        onOpenChange={() => undefined}
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
});
