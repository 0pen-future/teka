import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { AdjustmentDialog } from "../components/adjustment-dialog";
import type { ReviewRow } from "../schemas/billing-schemas";
import { billingHandlers, fixturePeriodOpen, resetBillingStore } from "./billing-handlers";

const row: ReviewRow = {
  invoice_id: "invoice-multi",
  student_id: "student-multi",
  contact_id: "contact-multi",
  student_name: "Nguyễn Văn An",
  contact_name: "Nguyễn Thị Bình",
  lines: [],
  opening_balance: 0,
  current_charge: 960000,
  adjustment_total: 0,
  total_due: 960000,
};

beforeEach(() => {
  resetBillingStore();
  server.use(...billingHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("AdjustmentDialog", () => {
  it("blocks submit while the reason is blank, mirroring the DB CHECK", async () => {
    signInAs(testPrimaryTeacher);
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(
      <AdjustmentDialog
        open
        onOpenChange={onOpenChange}
        periodId={fixturePeriodOpen.id}
        row={row}
      />,
    );

    const submitButton = screen.getByRole("button", { name: "Lưu điều chỉnh" });
    expect(submitButton).toBeDisabled();

    const amountInput = screen.getByLabelText("Số tiền điều chỉnh (đồng)");
    await user.clear(amountInput);
    await user.type(amountInput, "-50000");
    expect(submitButton).toBeDisabled();

    const reasonInput = screen.getByLabelText("Lý do");
    await user.type(reasonInput, "ab");
    expect(submitButton).toBeDisabled();

    await user.type(reasonInput, "c bù buổi nghỉ");
    expect(submitButton).toBeEnabled();
  });

  it("submits a signed adjustment and closes on success", async () => {
    signInAs(testPrimaryTeacher);
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(
      <AdjustmentDialog
        open
        onOpenChange={onOpenChange}
        periodId={fixturePeriodOpen.id}
        row={row}
      />,
    );

    const amountInput = screen.getByLabelText("Số tiền điều chỉnh (đồng)");
    await user.clear(amountInput);
    await user.type(amountInput, "-50000");
    const reasonInput = screen.getByLabelText("Lý do");
    await user.type(reasonInput, "Bù buổi nghỉ lễ");

    await user.click(screen.getByRole("button", { name: "Lưu điều chỉnh" }));

    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });
});
