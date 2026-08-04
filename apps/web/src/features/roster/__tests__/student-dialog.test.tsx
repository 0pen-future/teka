import { screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { StudentDialog } from "../components/student-dialog";
import { resetRosterStore, rosterHandlers } from "./roster-handlers";

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("StudentDialog", () => {
  it("renders exactly the three fields PRD R1's closed field list allows", async () => {
    renderWithProviders(<StudentDialog open onOpenChange={() => undefined} />);

    // Wait for the contact search combobox to mount before counting roles.
    await screen.findByRole("combobox");

    const textboxes = screen.getAllByRole("textbox");
    const comboboxes = screen.getAllByRole("combobox");
    expect(textboxes.length + comboboxes.length).toBe(3);

    expect(screen.getByRole("textbox", { name: "Họ và tên" })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Ghi chú phân biệt" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Người liên hệ" })).toBeInTheDocument();
  });
});
