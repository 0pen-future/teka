import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { NotificationsPage } from "../pages/notifications-page";
import {
  collectionsHandlers,
  contactTwoChildren,
  contactUnderpaid,
  fixturePeriod,
  resetCollectionsStore,
} from "./collections-handlers";

function renderNotificationsPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<NotificationsPage />, {
    route: `/notifications/${fixturePeriod.id}`,
    path: "/notifications/:periodId",
  });
}

beforeEach(() => {
  resetCollectionsStore();
  server.use(...collectionsHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("NotificationsPage", () => {
  it("shows an empty state with an explicit generate button and never auto-calls bulk on mount", async () => {
    renderNotificationsPage();

    // The empty state must be visible without any user interaction — proving
    // no effect fired the non-idempotent bulk-send call on mount.
    expect(await screen.findByText("Chưa có thông báo nào cho kỳ này.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tạo thông báo học phí" })).toBeInTheDocument();
    expect(screen.queryAllByRole("button", { name: "Sao chép" })).toHaveLength(0);
    expect(screen.queryByText(contactTwoChildren.full_name)).not.toBeInTheDocument();
  });

  it("renders exactly one message card per contact after an explicit generate click, never one per child", async () => {
    const user = userEvent.setup();
    renderNotificationsPage();

    // Wait for the ledger fetch to settle (empty state stable) before
    // clicking, so the click lands on the persistent empty-state button
    // rather than the transient header button shown while it's still loading.
    await screen.findByText("Chưa có thông báo nào cho kỳ này.");
    await user.click(screen.getByRole("button", { name: "Tạo thông báo học phí" }));

    // contactTwoChildren has two invoices (two children, two classes) but the
    // bulk-send response groups by contact, so its name must appear once.
    await waitFor(() => expect(screen.getAllByText(contactTwoChildren.full_name)).toHaveLength(1));

    // All four fixture contacts get a card for the "statements" purpose.
    expect(screen.getAllByRole("button", { name: "Sao chép" })).toHaveLength(4);
  });

  it("marks a contact's message as sent and reflects the status without affecting others", async () => {
    const user = userEvent.setup();
    renderNotificationsPage();

    await screen.findByText("Chưa có thông báo nào cho kỳ này.");
    await user.click(screen.getByRole("button", { name: "Tạo thông báo học phí" }));

    await waitFor(() => expect(screen.getAllByText(contactTwoChildren.full_name)).toHaveLength(1));
    expect(screen.queryByText("✓ Đã gửi")).not.toBeInTheDocument();

    // Fixture contacts are generated in the same order the store's invoices
    // list them: contactTwoChildren, contactSingleChildOwing, contactUnderpaid,
    // contactTwoChildrenOwing — so contactUnderpaid's card is index 2.
    const markSentButtons = screen.getAllByRole("button", { name: "Đã gửi" });
    expect(markSentButtons).toHaveLength(4);
    await user.click(markSentButtons[2]!);

    await waitFor(() => expect(screen.getAllByText("✓ Đã gửi")).toHaveLength(1));

    const updatedButtons = screen.getAllByRole("button", { name: "Đã gửi" });
    expect(updatedButtons[2]).toBeDisabled();
    // Every other contact's message stays untouched.
    expect(updatedButtons[0]).toBeEnabled();
    expect(updatedButtons[1]).toBeEnabled();
    expect(updatedButtons[3]).toBeEnabled();

    expect(screen.getByText(contactUnderpaid.full_name)).toBeInTheDocument();
  });
});
