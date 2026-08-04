import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { CollectionsPage } from "../pages/collections-page";
import {
  collectionsHandlers,
  contactSingleChildOwing,
  contactTwoChildren,
  contactUnderpaid,
  fixturePeriod,
  resetCollectionsStore,
} from "./collections-handlers";

function renderCollectionsPage(route = `/collections/${fixturePeriod.id}`) {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<CollectionsPage />, { route, path: "/collections/:periodId" });
}

beforeEach(() => {
  resetCollectionsStore();
  server.use(...collectionsHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("CollectionsPage", () => {
  it("defaults to the by-contact view without a ?view= param", async () => {
    renderCollectionsPage();

    expect(await screen.findByText(contactTwoChildren.full_name)).toBeInTheDocument();
    const contactTab = screen.getByRole("tab", { name: "Theo phụ huynh" });
    expect(contactTab).toHaveAttribute("aria-selected", "true");
  });

  it("reaches the unpaid filter in one interaction and reduces the row set", async () => {
    const user = userEvent.setup();
    renderCollectionsPage();

    await screen.findByText(contactTwoChildren.full_name);
    expect(screen.getByText(contactSingleChildOwing.full_name)).toBeInTheDocument();
    expect(screen.getByText(contactUnderpaid.full_name)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Chưa đóng" }));

    expect(await screen.findByText(contactSingleChildOwing.full_name)).toBeInTheDocument();
    expect(screen.queryByText(contactTwoChildren.full_name)).not.toBeInTheDocument();
    expect(screen.queryByText(contactUnderpaid.full_name)).not.toBeInTheDocument();
  });
});
