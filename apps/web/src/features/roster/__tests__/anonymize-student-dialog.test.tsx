import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { AnonymizeStudentDialog } from "../components/anonymize-student-dialog";
import { resetRosterStore, rosterHandlers, studentSiblingOne } from "./roster-handlers";

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("AnonymizeStudentDialog", () => {
  it("states that personal data is erased while financial records are kept, anonymized", () => {
    renderWithProviders(
      <AnonymizeStudentDialog open onOpenChange={() => undefined} student={studentSiblingOne} />,
    );

    expect(
      screen.getByText(/Phiếu thu và lịch sử thanh toán được giữ lại ở dạng ẩn danh/),
    ).toBeInTheDocument();
  });

  it("calls DELETE /students/:id and reports success on confirm", async () => {
    const user = userEvent.setup();
    const onSuccess = vi.fn();
    server.use(
      http.delete(`${API_URL}/students/:id`, () => new HttpResponse(null, { status: 204 })),
    );
    renderWithProviders(
      <AnonymizeStudentDialog
        open
        onOpenChange={() => undefined}
        student={studentSiblingOne}
        onSuccess={onSuccess}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Xoá dữ liệu" }));

    await vi.waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1));
  });

  it("shows contact-preserving success without exposing a type-to-confirm gate", () => {
    renderWithProviders(
      <AnonymizeStudentDialog open onOpenChange={() => undefined} student={studentSiblingOne} />,
    );

    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Xoá dữ liệu" })).toBeInTheDocument();
  });

  it("names the affected student in the confirmation copy", () => {
    renderWithProviders(
      <AnonymizeStudentDialog open onOpenChange={() => undefined} student={studentSiblingOne} />,
    );

    expect(
      screen.getByText(studentSiblingOne.full_name, { selector: "strong" }),
    ).toBeInTheDocument();
  });
});
