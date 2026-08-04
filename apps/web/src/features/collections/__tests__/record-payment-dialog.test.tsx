import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { RecordPaymentDialog } from "../components/record-payment-dialog";
import {
  collectionsHandlers,
  contactTwoChildrenOwing,
  fixturePeriod,
  invoiceStudentD1,
  invoiceStudentD2,
  resetCollectionsStore,
} from "./collections-handlers";

function renderDialog() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(
    <RecordPaymentDialog
      open
      onOpenChange={() => undefined}
      periodId={fixturePeriod.id}
      contactId={contactTwoChildrenOwing.id}
      contactName={contactTwoChildrenOwing.full_name}
    />,
  );
}

beforeEach(() => {
  resetCollectionsStore();
  server.use(...collectionsHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("RecordPaymentDialog", () => {
  it("blocks the reallocation submit while the edited allocation sum mismatches the payment amount, and enables it once the sum matches again", async () => {
    const user = userEvent.setup();
    renderDialog();

    const amountInput = screen.getByLabelText("Số tiền");
    await user.click(amountInput);
    await user.type(amountInput, "950000");
    await user.click(screen.getByRole("button", { name: "Ghi nhận" }));

    // Both children's amounts now show, prefilled by the server's D8 split
    // (500.000 / 450.000, one line per invoice — never merged into one).
    // `HvModal` portals its content to `document.body`, outside the render
    // container, so the allocation inputs are queried from `document`.
    await screen.findByText(/Đã ghi nhận/);
    const firstAllocationInput = await waitFor(() => {
      const input = document.querySelector<HTMLInputElement>(`#allocation-${invoiceStudentD1}`);
      expect(input).not.toBeNull();
      return input!;
    });
    const secondAllocationInput = document.querySelector<HTMLInputElement>(
      `#allocation-${invoiceStudentD2}`,
    )!;

    await user.clear(firstAllocationInput);
    await user.type(firstAllocationInput, "100000");

    // Mismatched sum (100.000 + 450.000 != 950.000): the correction button stays disabled.
    const submitButton = screen.getByRole("button", { name: "Cập nhật phân bổ" });
    expect(submitButton).toBeDisabled();

    // A different (but still valid, still non-default) split that sums back
    // to the payment amount re-enables the button without reverting to the
    // server's original default split.
    await user.clear(firstAllocationInput);
    await user.type(firstAllocationInput, "450000");
    await user.clear(secondAllocationInput);
    await user.type(secondAllocationInput, "500000");

    await waitFor(() => expect(submitButton).toBeEnabled());
    await user.click(submitButton);

    // The reallocation write lands and the dialog reflects the confirmed split.
    await waitFor(() => expect(screen.getByRole("button", { name: "Xong" })).toBeInTheDocument());
  });
});
