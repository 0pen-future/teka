import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
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

  it("keeps Thu tiền but hides Nhắc nợ from a plain member (D8)", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
      ),
    );
    renderCollectionsPage();

    await screen.findByText(contactTwoChildren.full_name);
    // Payments stay member work; creating reminder sends does not.
    expect(screen.getAllByRole("button", { name: "Thu tiền" }).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Nhắc nợ" })).not.toBeInTheDocument();
  });

  it("offers Nhắc nợ to the owner by default", async () => {
    renderCollectionsPage();

    // findAll: the button appears only once /centers/me resolves owner-shaped.
    expect((await screen.findAllByRole("button", { name: "Nhắc nợ" })).length).toBeGreaterThan(0);
  });
});
